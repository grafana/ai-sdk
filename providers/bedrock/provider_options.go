package bedrock

import (
	"bytes"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

// providerOptionKeys lists the namespace keys we accept for Bedrock-specific
// options. Upstream historically used `bedrock`; current docs use
// `amazonBedrock`. Both are honored at read time so callers can pick either.
var providerOptionKeys = []string{"amazonBedrock", "bedrock"}

// resolveBedrockOption resolves a typed Bedrock provider option from a
// ProviderOptions map, checking both the modern (`amazonBedrock`) and legacy
// (`bedrock`) namespace keys in order. It returns the parsed value, whether a
// value was found, and any decode error.
//
// Unlike the previous best-effort readers, a malformed option (e.g. invalid
// JSON in a RawProviderOption) surfaces as an error so the call fails loudly
// rather than silently falling back to zero values. This matches the
// anthropic provider's applyProviderOptions behavior.
func resolveBedrockOption[T any](opts provider.ProviderOptions) (T, bool, error) {
	var zero T
	for _, key := range providerOptionKeys {
		option, ok := opts[key]
		if !ok || option == nil {
			continue
		}
		if typed, ok := any(option).(*T); ok {
			if typed == nil {
				continue
			}
			return *typed, true, nil
		}
		if raw, ok := option.(provider.RawProviderOption); ok &&
			(len(raw.Raw) == 0 || bytes.Equal(bytes.TrimSpace(raw.Raw), []byte("null"))) {
			continue
		}
		v, ok, err := provider.ResolveOption[T](opts, key)
		if err != nil {
			return zero, true, fmt.Errorf("bedrock: invalid provider options for %q: %w", key, err)
		}
		if ok {
			return v, true, nil
		}
	}
	return zero, false, nil
}

// readBedrockOptions resolves the typed Bedrock request options, checking both
// modern (`amazonBedrock`) and legacy (`bedrock`) keys. Returns the zero value
// when neither key is set, and an error when a present option fails to decode.
func readBedrockOptions(opts provider.ProviderOptions) (BedrockOptions, bool, error) {
	return resolveBedrockOption[BedrockOptions](opts)
}

// extractCachePoint reads a cache-point configuration from a message or
// content-part's ProviderOptions. Returns nil when no cache point is
// configured, and an error when the option is present but malformed.
func extractCachePoint(opts provider.ProviderOptions) (*cachePoint, error) {
	bo, ok, err := readBedrockOptions(opts)
	if err != nil {
		return nil, err
	}
	if !ok || bo.CachePoint == nil {
		return nil, nil
	}
	typ := bo.CachePoint.Type
	if typ == "" {
		typ = "default"
	}
	return &cachePoint{Type: typ, TTL: bo.CachePoint.TTL}, nil
}

// shouldEnableCitations returns true when a file part's ProviderOptions enable
// Bedrock document citations. Returns an error when the option is present but
// malformed.
func shouldEnableCitations(opts provider.ProviderOptions) (bool, error) {
	fpo, ok, err := resolveBedrockOption[FilePartOptions](opts)
	if err != nil {
		return false, err
	}
	if !ok || fpo.Citations == nil {
		return false, nil
	}
	return fpo.Citations.Enabled, nil
}

// readReasoningMetadata pulls the Bedrock reasoning signature/redacted-data out
// of a content part's ProviderOptions. Used when forwarding assistant reasoning
// content back to Bedrock without breaking signed thinking blocks. Returns an
// error when the option is present but malformed.
type reasoningReplayMetadata struct {
	Signature       *string `json:"signature,omitempty"`
	RedactedData    *string `json:"redactedData,omitempty"`
	RedactedContent *string `json:"redactedContent,omitempty"`
}

func readReasoningMetadata(opts provider.ProviderOptions) (reasoningReplayMetadata, bool, error) {
	// Keep the public typed options compatible while retaining presence when
	// replaying raw metadata (where a present empty string is meaningful).
	for _, key := range providerOptionKeys {
		var typed *ReasoningMetadata
		switch value := opts[key].(type) {
		case ReasoningMetadata:
			typed = &value
		case *ReasoningMetadata:
			typed = value
		}
		if typed != nil {
			copyOptions := make(provider.ProviderOptions, len(opts))
			for k, value := range opts {
				copyOptions[k] = value
			}
			copyOptions[key] = provider.RawProviderOption{Key: key, Raw: jsonRawOrZero(typed)}
			opts = copyOptions
		}
	}
	rm, ok, err := resolveBedrockOption[reasoningReplayMetadata](opts)
	if err != nil {
		return reasoningReplayMetadata{}, false, err
	}
	if !ok || (rm.Signature == nil && rm.RedactedData == nil && rm.RedactedContent == nil) {
		return reasoningReplayMetadata{}, false, nil
	}
	return rm, true, nil
}
