package execution

import (
	"encoding/json"
	"errors"
	"maps"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// ControlNamespace is the call-level provider-option namespace that carries
// Gateway host controls, such as BYOK credentials. The runtime removes it before
// selection and hands it to the selector, so it never reaches a model.
// Namespaces are compared exactly, because they are case-significant.
const ControlNamespace = "gateway"

// ErrProtectedCallHeader reports a credential-bearing call header.
var ErrProtectedCallHeader = errors.New("execution: protected call header")

// protectedCallHeaders never cross from a request into a backend call.
// Providers apply call headers after their own authorization header, so a
// caller-supplied entry here would choose the credential, or the tenant,
// presented upstream.
var protectedCallHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"x-access-token":      {},
	"x-grafana-id":        {},
	"x-api-key":           {},
	"api-key":             {},
	"openai-api-key":      {},
	"anthropic-api-key":   {},
	"openai-organization": {},
	"openai-project":      {},
	"x-scope-orgid":       {},
	"x-cloud-org-id":      {},
	"x-access-policy-id":  {},
}

// isProtectedCallHeader reports whether a call header name, compared without
// case, carries credentials.
func isProtectedCallHeader(name string) bool {
	_, protected := protectedCallHeaders[strings.ToLower(name)]
	return protected
}

// applyPolicy refuses protected call headers, then removes the call-level
// control namespace and returns its raw JSON, which is nil when absent. The
// caller's options are not modified.
func applyPolicy(options provider.CallOptions) (provider.CallOptions, json.RawMessage, error) {
	for name := range options.Headers {
		if isProtectedCallHeader(name) {
			return provider.CallOptions{}, nil, ErrProtectedCallHeader
		}
	}
	value, ok := options.ProviderOptions[ControlNamespace]
	if !ok {
		return options, nil, nil
	}
	controls, err := rawOption(value)
	if err != nil {
		return provider.CallOptions{}, nil, err
	}
	remaining := maps.Clone(options.ProviderOptions)
	delete(remaining, ControlNamespace)
	if len(remaining) == 0 {
		remaining = nil
	}
	options.ProviderOptions = remaining
	return options, controls, nil
}

func rawOption(value provider.ProviderOption) (json.RawMessage, error) {
	if raw, ok := value.(provider.RawProviderOption); ok {
		return raw.Raw, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, ErrReservedProviderOptions
	}
	return encoded, nil
}
