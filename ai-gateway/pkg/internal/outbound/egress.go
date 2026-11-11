package outbound

import (
	"errors"
	"net/netip"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

// MaxEgressEntries bounds each egress list setting.
const MaxEgressEntries = 256

const domainWildcard = "*."

var (
	// ErrEndpointHost reports an endpoint host that is neither an IP address
	// nor a DNS name, such as a numeric form a resolver may read as IPv4.
	ErrEndpointHost = errors.New("gateway outbound: endpoint host must be a DNS name or an IP address")

	errEgressEntry      = errors.New("gateway outbound: egress entries must be IP addresses, canonical CIDR prefixes, hostnames or *. domains")
	errEgressDuplicate  = errors.New("gateway outbound: egress entries must be unique")
	errEgressEntryCount = errors.New("gateway outbound: too many egress entries")
)

var (
	refusedIPv4 = mustPrefixes(
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	)
	globalUnicastIPv6 = netip.MustParsePrefix("2000::/3")
	refusedIPv6       = mustPrefixes("2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20")
	nat64WellKnown    = netip.MustParsePrefix("64:ff9b::/96")
)

func mustPrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, len(values))
	for i, value := range values {
		prefixes[i] = netip.MustParsePrefix(value)
	}
	return prefixes
}

// EgressList is one operator list of egress destinations: addresses,
// prefixes, hostnames and domain wildcards.
type EgressList struct {
	prefixes []netip.Prefix
	hosts    []string
	domains  []string
}

// ParseEgressList parses comma-separated egress entries: an IP address, a
// canonical CIDR prefix, a hostname, or "*." and a domain, which matches its
// subdomains only. An empty value is an empty list. Errors never contain the
// value.
func ParseEgressList(raw string) (EgressList, error) {
	var list EgressList
	if raw == "" {
		return list, nil
	}
	seen := make(map[string]struct{})
	for entry := range strings.SplitSeq(raw, ",") {
		if len(seen) == MaxEgressEntries {
			return EgressList{}, errEgressEntryCount
		}
		key, err := list.add(strings.TrimSpace(entry))
		if err != nil {
			return EgressList{}, err
		}
		if _, ok := seen[key]; ok {
			return EgressList{}, errEgressDuplicate
		}
		seen[key] = struct{}{}
	}
	return list, nil
}

func (list *EgressList) add(entry string) (string, error) {
	if strings.Contains(entry, "/") {
		prefix, err := netip.ParsePrefix(entry)
		if err != nil || prefix != prefix.Masked() || prefix.Addr().Is4In6() {
			return "", errEgressEntry
		}
		list.prefixes = append(list.prefixes, prefix)
		return prefix.String(), nil
	}
	if addr, err := netip.ParseAddr(entry); err == nil {
		if addr.Zone() != "" || addr.Is4In6() {
			return "", errEgressEntry
		}
		prefix := netip.PrefixFrom(addr, addr.BitLen())
		list.prefixes = append(list.prefixes, prefix)
		return prefix.String(), nil
	}
	if domain, ok := strings.CutPrefix(entry, domainWildcard); ok {
		domain = normalizeHost(domain)
		if !ValidHostname(domain) {
			return "", errEgressEntry
		}
		list.domains = append(list.domains, "."+domain)
		return domainWildcard + domain, nil
	}
	host := normalizeHost(entry)
	if !ValidHostname(host) {
		return "", errEgressEntry
	}
	list.hosts = append(list.hosts, host)
	return host, nil
}

func (list EgressList) matchesHost(host string) bool {
	host = normalizeHost(host)
	if slices.Contains(list.hosts, host) {
		return true
	}
	return slices.ContainsFunc(list.domains, func(domain string) bool { return strings.HasSuffix(host, domain) })
}

func (list EgressList) containsAddr(addrs ...netip.Addr) bool {
	return slices.ContainsFunc(list.prefixes, func(prefix netip.Prefix) bool {
		return slices.ContainsFunc(addrs, prefix.Contains)
	})
}

// EgressPolicy decides which destinations the egress client may connect to.
// A deny entry always wins; an allow entry lifts the built-in refusal of
// non-public addresses, except unspecified and multicast ones. The zero
// policy applies only the built-in refusal.
type EgressPolicy struct {
	Allow EgressList
	Deny  EgressList
}

// CheckHost checks an endpoint host without resolving it: a deny-listed
// hostname, an IP literal the policy refuses, and a host that is neither an
// IP address nor a DNS name fail. A host that passes can still be refused
// when connecting.
func (policy EgressPolicy) CheckHost(host string) error {
	if addr, err := netip.ParseAddr(host); err == nil {
		if !policy.permits(addr, false) {
			return execution.ErrEgressRefused
		}
		return nil
	}
	if !ValidHostname(normalizeHost(host)) {
		return ErrEndpointHost
	}
	if policy.Deny.matchesHost(host) {
		return execution.ErrEgressRefused
	}
	return nil
}

// permits reports whether the egress client may connect to addr. hostAllowed
// reports that the endpoint's hostname matches an allow entry.
func (policy EgressPolicy) permits(addr netip.Addr, hostAllowed bool) bool {
	if addr.Zone() != "" {
		return false
	}
	addr = addr.Unmap()
	effective := embeddedIPv4(addr)
	if policy.Deny.containsAddr(addr, effective) {
		return false
	}
	if effective.IsUnspecified() || effective.IsMulticast() {
		return false
	}
	if hostAllowed || policy.Allow.containsAddr(addr, effective) {
		return true
	}
	return globallyReachable(effective)
}

// embeddedIPv4 returns the IPv4 address a NAT64 well-known-prefix address
// embeds, or addr itself.
func embeddedIPv4(addr netip.Addr) netip.Addr {
	if !nat64WellKnown.Contains(addr) {
		return addr
	}
	bytes := addr.As16()
	return netip.AddrFrom4([4]byte(bytes[12:]))
}

func globallyReachable(addr netip.Addr) bool {
	if addr.Is4() {
		return !slices.ContainsFunc(refusedIPv4, func(prefix netip.Prefix) bool { return prefix.Contains(addr) })
	}
	return globalUnicastIPv6.Contains(addr) && !slices.ContainsFunc(refusedIPv6, func(prefix netip.Prefix) bool { return prefix.Contains(addr) })
}

// ValidHostname reports whether host is a lowercase ASCII DNS name whose last
// label starts with a letter, so no resolver reads it as a numeric IPv4 form
// such as 127.1, 2130706433 or 0x7f000001.
func ValidHostname(host string) bool {
	if len(validation.IsDNS1123Subdomain(host)) != 0 {
		return false
	}
	labels := strings.Split(host, ".")
	if slices.ContainsFunc(labels, func(label string) bool { return len(validation.IsDNS1123Label(label)) != 0 }) {
		return false
	}
	last := labels[len(labels)-1]
	return last[0] >= 'a' && last[0] <= 'z'
}

// normalizeHost lowercases host and removes one trailing dot.
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}
