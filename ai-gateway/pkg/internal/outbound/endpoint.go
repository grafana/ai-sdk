package outbound

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// MaxCustomEndpoints bounds the custom endpoint list setting.
const MaxCustomEndpoints = 256

var (
	// ErrEndpointForm reports a custom endpoint that is not an absolute URL
	// with an ASCII host and no userinfo, query or fragment, or whose path has
	// a backslash, a "." or ".." segment, a ";" or an encoded "/", backslash
	// or "%".
	ErrEndpointForm = errors.New("gateway outbound: endpoint must be an absolute URL without userinfo, query, fragment, dot segments or encoded separators")
	// ErrEndpointNotListed reports a custom endpoint the operator doesn't list.
	ErrEndpointNotListed = errors.New("gateway outbound: endpoint is not a listed custom endpoint")

	errEndpointEntry      = errors.New("gateway outbound: custom endpoints must be http or https base URLs with an ASCII host and no userinfo, query, fragment, wildcard or dot segments")
	errEndpointDuplicate  = errors.New("gateway outbound: custom endpoints must be unique")
	errEndpointEntryCount = fmt.Errorf("gateway outbound: at most %d custom endpoints are allowed", MaxCustomEndpoints)
)

// endpoint is a base URL normalized for matching: lowercase scheme and host,
// the port as written, and the escaped path without a trailing "/".
type endpoint struct {
	scheme, host, port, path string
}

func (e endpoint) key() string { return e.scheme + "://" + e.host + ":" + e.port + e.path }

// matches reports whether candidate is e or continues its path after a "/".
func (e endpoint) matches(candidate endpoint) bool {
	if candidate.scheme != e.scheme || candidate.host != e.host || candidate.port != e.port {
		return false
	}
	return candidate.path == e.path || e.path == "" || strings.HasPrefix(candidate.path, e.path+"/")
}

// parseEndpoint parses and normalizes a base URL, refusing one that isn't an
// absolute http or https URL with an ASCII host and no userinfo, query or
// fragment. It must not contain "\", and no path segment may decode to "."
// or "..", or to a value containing "/", "\", "%" or ";": the provider SDKs
// decode the base URL's path and resolve dot segments when they join it with
// their own, so an encoded separator, a doubly encoded dot segment at a server
// that decodes twice, or a dot segment with a path parameter at a server that
// strips it, could move a request outside a listed path.
func parseEndpoint(raw string) (endpoint, *url.URL, bool) {
	if strings.Contains(raw, "\\") {
		return endpoint{}, nil, false
	}
	parsed, err := ValidateEndpoint(raw)
	if err != nil || !ascii(parsed.Hostname()) {
		return endpoint{}, nil, false
	}
	path := strings.TrimSuffix(parsed.EscapedPath(), "/")
	for segment := range strings.SplitSeq(path, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\%;") {
			return endpoint{}, nil, false
		}
	}
	return endpoint{scheme: strings.ToLower(parsed.Scheme), host: strings.ToLower(parsed.Hostname()), port: parsed.Port(), path: path}, parsed, true
}

// CustomEndpoints is the operator's list of base URLs that stack-catalog
// providers and request accounts may use as a custom endpoint. The zero
// value lists none.
type CustomEndpoints struct {
	entries []endpoint
}

// ParseCustomEndpoints parses comma-separated base URLs. Each must be an
// absolute http or https URL with an ASCII host and no userinfo, query,
// fragment, "*" or "." or ".." path segment. An empty value is an empty
// list. Errors never contain the value.
func ParseCustomEndpoints(raw string) (CustomEndpoints, error) {
	var list CustomEndpoints
	if raw == "" {
		return list, nil
	}
	seen := make(map[string]struct{})
	for entry := range strings.SplitSeq(raw, ",") {
		if len(list.entries) == MaxCustomEndpoints {
			return CustomEndpoints{}, errEndpointEntryCount
		}
		entry = strings.TrimSpace(entry)
		parsed, _, ok := parseEndpoint(entry)
		if !ok || strings.Contains(entry, "*") {
			return CustomEndpoints{}, errEndpointEntry
		}
		if _, ok := seen[parsed.key()]; ok {
			return CustomEndpoints{}, errEndpointDuplicate
		}
		seen[parsed.key()] = struct{}{}
		list.entries = append(list.entries, parsed)
	}
	return list, nil
}

// Check checks a customer-chosen endpoint without resolving it. An endpoint
// equal to native, the provider type's native endpoint, counts as unset:
// Check returns "". Scheme and host compare case-insensitively and a trailing
// "/" is ignored. Any other endpoint must match a listed entry, and Check
// returns it normalized. It fails with [ErrEndpointForm] or
// [ErrEndpointNotListed], neither of which contains the URL.
func (list CustomEndpoints) Check(raw, native string) (string, error) {
	candidate, parsed, ok := parseEndpoint(raw)
	if !ok {
		return "", ErrEndpointForm
	}
	if isNative(parsed, native) {
		return "", nil
	}
	for _, entry := range list.entries {
		if entry.matches(candidate) {
			return parsed.String(), nil
		}
	}
	return "", ErrEndpointNotListed
}

func isNative(endpoint *url.URL, native string) bool {
	if native == "" {
		return false
	}
	want, err := url.Parse(native)
	if err != nil {
		return false
	}
	return strings.EqualFold(endpoint.Scheme, want.Scheme) && strings.EqualFold(endpoint.Host, want.Host) &&
		strings.TrimSuffix(endpoint.EscapedPath(), "/") == strings.TrimSuffix(want.EscapedPath(), "/")
}

func ascii(value string) bool {
	return !strings.ContainsFunc(value, func(r rune) bool { return r > 0x7f })
}
