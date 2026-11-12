package outbound

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

func mustPolicy(t *testing.T, allow, deny string) EgressPolicy {
	t.Helper()
	allowList, err := ParseEgressList(allow)
	require.NoError(t, err)
	denyList, err := ParseEgressList(deny)
	require.NoError(t, err)
	return EgressPolicy{Allow: allowList, Deny: denyList}
}

func TestEgressPolicy_BuiltInRanges(t *testing.T) {
	refused := []string{
		"0.0.0.0", "0.255.255.255", "10.0.0.0", "10.255.255.255", "100.64.0.0", "100.127.255.255",
		"127.0.0.0", "127.255.255.255", "169.254.0.0", "169.254.169.254", "169.254.255.255",
		"172.16.0.0", "172.31.255.255", "192.0.0.0", "192.0.0.255", "192.0.2.0", "192.0.2.255",
		"192.88.99.0", "192.88.99.255", "192.168.0.0", "192.168.255.255", "198.18.0.0", "198.19.255.255",
		"198.51.100.0", "198.51.100.255", "203.0.113.0", "203.0.113.255", "224.0.0.0", "239.255.255.255",
		"240.0.0.0", "255.255.255.255",
		"::", "::1", "::127.0.0.1", "100::1", "fc00::", "fd00:ec2::254", "fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
		"fe80::1", "fec0::1", "ff02::1", "5f00::1", "1fff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "4000::",
		"2001::", "2001:1ff:ffff:ffff:ffff:ffff:ffff:ffff", "2001:db8::", "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff",
		"2002::", "2002:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "2002:a00:1::", "3fff::", "3fff:fff:ffff:ffff:ffff:ffff:ffff:ffff",
		"::ffff:127.0.0.1", "::ffff:169.254.169.254", "::ffff:10.0.0.1", "::ffff:0.0.0.0",
		"64:ff9b::a00:1", "64:ff9b::7f00:1", "64:ff9b:1::1", "64:ff9b::e000:1",
	}
	permitted := []string{
		"8.8.8.8", "1.1.1.1", "9.255.255.255", "11.0.0.0", "100.63.255.255", "100.128.0.0", "126.255.255.255",
		"128.0.0.0", "169.253.255.255", "169.255.0.0", "172.15.255.255", "172.32.0.0", "192.0.1.0",
		"192.88.98.255", "192.167.255.255", "192.169.0.0", "198.17.255.255", "198.20.0.0", "223.255.255.255",
		"2000::", "2001:200::", "2001:4860:4860::8888", "2606:4700::1111", "2003::", "3ffe:ffff::", "3fff:1000::",
		"::ffff:8.8.8.8", "64:ff9b::808:808",
	}
	policy := EgressPolicy{}
	for _, value := range refused {
		t.Run("refused "+value, func(t *testing.T) {
			assert.False(t, policy.permits(netip.MustParseAddr(value), false))
		})
	}
	for _, value := range permitted {
		t.Run("permitted "+value, func(t *testing.T) {
			assert.True(t, policy.permits(netip.MustParseAddr(value), false))
		})
	}
	t.Run("zones are refused", func(t *testing.T) {
		assert.False(t, policy.permits(netip.MustParseAddr("fe80::1%eth0"), false))
		assert.False(t, mustPolicy(t, "fe80::/10", "").permits(netip.MustParseAddr("fe80::1%eth0"), true))
	})
}

func TestEgressPolicy_Precedence(t *testing.T) {
	tests := []struct {
		name        string
		allow, deny string
		addr        string
		hostAllowed bool
		permitted   bool
	}{
		{name: "allow prefix lifts a built-in refusal", allow: "10.20.0.0/16", addr: "10.20.1.2", permitted: true},
		{name: "allow prefix outside the address", allow: "10.20.0.0/16", addr: "10.21.0.1"},
		{name: "allow address", allow: "127.0.0.1", addr: "127.0.0.1", permitted: true},
		{name: "allow address matches its mapped form", allow: "127.0.0.1", addr: "::ffff:127.0.0.1", permitted: true},
		{name: "allow lifts NAT64 to a private address", allow: "10.0.0.0/8", addr: "64:ff9b::a00:1", permitted: true},
		{name: "allowed hostname lifts a built-in refusal", addr: "10.0.0.1", hostAllowed: true, permitted: true},
		{name: "deny wins over an allowed hostname", deny: "10.0.0.0/8", addr: "10.0.0.1", hostAllowed: true},
		{name: "deny wins over an allow prefix", allow: "10.0.0.0/8", deny: "10.1.0.0/16", addr: "10.1.0.1"},
		{name: "deny a public address", deny: "203.0.113.7/32", addr: "203.0.113.7"},
		{name: "deny a public prefix", deny: "8.8.8.0/24", addr: "8.8.8.8"},
		{name: "deny matches the embedded address", deny: "8.8.8.0/24", addr: "64:ff9b::808:808"},
		{name: "deny matches the mapped address", deny: "8.8.8.8", addr: "::ffff:8.8.8.8"},
		{name: "deny IPv6", deny: "2606:4700::/32", addr: "2606:4700::1111"},
		{name: "allow everything cannot reach unspecified", allow: "0.0.0.0/0", addr: "0.0.0.0"},
		{name: "allowed hostname cannot reach unspecified", addr: "::", hostAllowed: true},
		{name: "allow cannot reach multicast", allow: "224.0.0.0/4", addr: "224.0.0.1"},
		{name: "allow cannot reach IPv6 multicast", allow: "ff00::/8", addr: "ff02::1"},
		{name: "allow loopback", allow: "::1", addr: "::1", permitted: true},
		{name: "allow is not exclusive", allow: "10.0.0.0/8", addr: "8.8.8.8", permitted: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			policy := mustPolicy(t, tc.allow, tc.deny)
			assert.Equal(t, tc.permitted, policy.permits(netip.MustParseAddr(tc.addr), tc.hostAllowed))
		})
	}
}

func TestEgressList_MatchesHost(t *testing.T) {
	list, err := ParseEgressList("llm.corp.example, *.internal.example, Mixed.Example.")
	require.NoError(t, err)
	for host, matches := range map[string]bool{
		"llm.corp.example":      true,
		"LLM.corp.example.":     true,
		"a.llm.corp.example":    false,
		"corp.example":          false,
		"a.internal.example":    true,
		"a.b.internal.example":  true,
		"internal.example":      false,
		"xinternal.example":     false,
		"mixed.example":         true,
		"internal.example.evil": false,
	} {
		t.Run(host, func(t *testing.T) {
			assert.Equal(t, matches, list.matchesHost(host))
		})
	}
}

func TestParseEgressList(t *testing.T) {
	t.Run("valid entries", func(t *testing.T) {
		for _, raw := range []string{"", "10.20.0.0/16,llm.corp.example", "*.internal.example, 203.0.113.7", "fd12::/48,::1,fake-anthropic", "0.0.0.0/0,::/0"} {
			_, err := ParseEgressList(raw)
			assert.NoError(t, err, raw)
		}
	})
	invalid := []string{
		" ", "10.20.0.5/16", "::ffff:10.0.0.0/104", "::ffff:10.0.0.1", "fe80::1%eth0", "*.", "*", "a,,b", "a,",
		"10.0.0.0/33", "llm.example.com:443", "https://llm.example.com", "127.1", "2130706433", "0x7f000001",
		"llm_corp.example", "*.*.example", "llm.ex ample", "ünïcode.example", "a,a", "10.0.0.1,10.0.0.1/32", "A.example,a.example.",
		strings.Repeat("a", 64) + ".example",
	}
	for _, raw := range invalid {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseEgressList(raw)
			require.Error(t, err)
			for _, part := range strings.Split(raw, ",") {
				if part = strings.TrimSpace(part); len(part) > 3 {
					assert.NotContains(t, err.Error(), part)
				}
			}
		})
	}
	t.Run("entry bound", func(t *testing.T) {
		entries := make([]string, maxEgressEntries+1)
		for i := range entries {
			entries[i] = fmt.Sprintf("host-%d.example", i)
		}
		_, err := ParseEgressList(strings.Join(entries[:maxEgressEntries], ","))
		require.NoError(t, err)
		_, err = ParseEgressList(strings.Join(entries, ","))
		require.ErrorIs(t, err, errEgressEntryCount)
	})
}

func TestEgressPolicy_CheckHost(t *testing.T) {
	policy := mustPolicy(t, "10.20.0.0/16", "denied.example,*.blocked.example,203.0.113.7")
	tests := []struct {
		host string
		want error
	}{
		{host: "llm.example.com"},
		{host: "LLM.Example.com."},
		{host: "fake-anthropic"},
		{host: "8.8.8.8"},
		{host: "2606:4700::1111"},
		{host: "10.20.0.1"},
		{host: "127.0.0.1", want: execution.ErrEgressRefused},
		{host: "169.254.169.254", want: execution.ErrEgressRefused},
		{host: "::ffff:10.0.0.1", want: execution.ErrEgressRefused},
		{host: "::1", want: execution.ErrEgressRefused},
		{host: "fe80::1%eth0", want: execution.ErrEgressRefused},
		{host: "203.0.113.7", want: execution.ErrEgressRefused},
		{host: "denied.example", want: execution.ErrEgressRefused},
		{host: "Denied.Example.", want: execution.ErrEgressRefused},
		{host: "a.blocked.example", want: execution.ErrEgressRefused},
		{host: "127.1", want: ErrEndpointHost},
		{host: "2130706433", want: ErrEndpointHost},
		{host: "0x7f000001", want: ErrEndpointHost},
		{host: "0x7f.1", want: ErrEndpointHost},
		{host: "017700000001", want: ErrEndpointHost},
		{host: "llm.1example", want: ErrEndpointHost},
		{host: "llm_corp.example", want: ErrEndpointHost},
		{host: "ünïcode.example", want: ErrEndpointHost},
		{host: "", want: ErrEndpointHost},
	}
	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			err := policy.CheckHost(tc.host)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.want)
		})
	}
}
