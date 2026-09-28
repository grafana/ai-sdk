package bedrock

import (
	"bytes"
	"encoding/json"
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

type anthropicCallOptions struct {
	StructuredOutputMode   StructuredOutputMode `json:"structuredOutputMode,omitempty"`
	DisableParallelToolUse *bool                `json:"disableParallelToolUse,omitempty"`
}

func readAnthropicCallOptions(opts provider.ProviderOptions) (anthropicCallOptions, error) {
	var value anthropicCallOptions
	option := opts["anthropic"]
	if option == nil {
		return value, nil
	}
	var data []byte
	if raw, ok := option.(provider.RawProviderOption); ok {
		data = raw.Raw
	} else {
		var err error
		data, err = json.Marshal(option)
		if err != nil {
			return value, fmt.Errorf("bedrock: marshaling anthropic provider options: %w", err)
		}
	}
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return value, nil
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("bedrock: invalid provider options for %q: %w", "anthropic", err)
	}
	return value, nil
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

func readPartOptions(opts provider.ProviderOptions) (map[string]json.RawMessage, error) {
	for _, key := range providerOptionKeys {
		option := opts[key]
		if option == nil {
			continue
		}
		var data []byte
		if raw, ok := option.(provider.RawProviderOption); ok {
			data = raw.Raw
		} else {
			var err error
			data, err = json.Marshal(option)
			if err != nil {
				return nil, fmt.Errorf("bedrock: marshaling provider options for %q: %w", key, err)
			}
		}
		data = bytes.TrimSpace(data)
		if len(data) == 0 || bytes.Equal(data, []byte("null")) {
			continue
		}
		var fields map[string]json.RawMessage
		if len(data) < 2 || data[0] != '{' || json.Unmarshal(data, &fields) != nil {
			return nil, fmt.Errorf("bedrock: invalid provider options for %q: expected JSON object", key)
		}
		return fields, nil
	}
	return nil, nil
}

func readPartField[T any](fields map[string]json.RawMessage, key string) (T, bool, error) {
	var value T
	raw, ok := fields[key]
	if !ok {
		return value, false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return value, true, fmt.Errorf("bedrock: invalid %s: null is not allowed", key)
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, true, fmt.Errorf("bedrock: invalid %s: %w", key, err)
	}
	return value, true, nil
}

func readTextGuardContent(opts provider.ProviderOptions) (bool, *[]GuardContentQualifier, error) {
	fields, err := readPartOptions(opts)
	if err != nil {
		return false, nil, err
	}
	guarded, _, err := readPartField[bool](fields, "guardContent")
	if err != nil {
		return false, nil, err
	}
	qualifiers, present, err := readPartField[[]GuardContentQualifier](fields, "guardContentQualifiers")
	if err != nil {
		return false, nil, err
	}
	if present {
		for _, qualifier := range qualifiers {
			switch qualifier {
			case GuardContentQualifierGroundingSource, GuardContentQualifierQuery, GuardContentQualifierGuardContent:
			default:
				return false, nil, fmt.Errorf("bedrock: invalid guardContentQualifiers value %q", qualifier)
			}
		}
	}
	if !present {
		return guarded, nil, nil
	}
	return guarded, &qualifiers, nil
}

func readImageGuardContent(opts provider.ProviderOptions) (bool, error) {
	fields, err := readPartOptions(opts)
	if err != nil {
		return false, err
	}
	guarded, _, err := readPartField[bool](fields, "guardContent")
	return guarded, err
}

// shouldEnableCitations returns true when a file part's ProviderOptions enable
// Bedrock document citations. Returns an error when the option is present but
// malformed.
func shouldEnableCitations(opts provider.ProviderOptions) (bool, error) {
	fields, err := readPartOptions(opts)
	if err != nil {
		return false, err
	}
	raw, ok := fields["citations"]
	if !ok {
		return false, nil
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return false, fmt.Errorf("bedrock: invalid provider options for citations: expected JSON object")
	}
	var citations map[string]json.RawMessage
	if err := json.Unmarshal(raw, &citations); err != nil {
		return false, fmt.Errorf("bedrock: invalid provider options for citations: %w", err)
	}
	enabled, present, err := readPartField[bool](citations, "enabled")
	if err != nil {
		return false, fmt.Errorf("bedrock: invalid provider options for citations: %w", err)
	}
	if !present {
		return false, fmt.Errorf("bedrock: invalid provider options for citations: enabled is required")
	}
	return enabled, nil
}

// readReasoningMetadata pulls the Bedrock reasoning signature/redacted-data out
// of a content part's ProviderOptions. Used when forwarding assistant reasoning
// content back to Bedrock without breaking signed thinking blocks. Returns an
// error when the option is present but malformed.
func readReasoningMetadata(opts provider.ProviderOptions) (ReasoningMetadata, bool, error) {
	rm, ok, err := resolveBedrockOption[ReasoningMetadata](opts)
	if err != nil {
		return ReasoningMetadata{}, false, err
	}
	if !ok || (rm.Signature == "" && rm.RedactedData == "") {
		return ReasoningMetadata{}, false, nil
	}
	return rm, true, nil
}
