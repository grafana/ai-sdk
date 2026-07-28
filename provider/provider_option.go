package provider

import (
	"encoding/json"
	"fmt"
)

// ProviderOption is a typed provider option value. Each provider defines
// concrete types implementing this interface (e.g., AnthropicOptions).
// The ProviderKey identifies the provider namespace (e.g., "anthropic").
type ProviderOption interface {
	ProviderKey() string
}

// RawProviderOption wraps opaque JSON data as a ProviderOption.
// Used when genuine JSON arrives from a wire boundary (e.g., ProviderMetadata
// from a previous SSE response converted back to ProviderOptions, or any
// ProviderOptions value decoded from the gateway/providerwire JSON+SSE transport).
type RawProviderOption struct {
	Key string
	Raw json.RawMessage
}

func (r RawProviderOption) ProviderKey() string { return r.Key }

// ProviderOptions is the canonical map type for provider-specific option
// values, keyed by provider name. It carries custom JSON marshalers so it
// round-trips losslessly through encoding/json.
//
// On Marshal, each value is serialized via [json.Marshal] of its concrete
// ProviderOption implementation: typed providers (e.g. AnthropicOptions)
// emit their own JSON shape; [RawProviderOption] writes its Raw bytes
// directly. The wire JSON is `{"<key>": <option JSON>, ...}`.
//
// On Unmarshal, the input JSON is decoded as a `map[string]json.RawMessage`
// and every entry is wrapped as `RawProviderOption{Key: k, Raw: v}`. This
// is intentionally asymmetric: a typed option goes out as its concrete
// struct and comes back as a [RawProviderOption]; consumers reach the typed
// view via [ResolveOption].
type ProviderOptions map[string]ProviderOption

// MarshalJSON implements [json.Marshaler]. See the [ProviderOptions] doc
// comment for the wire shape.
func (p ProviderOptions) MarshalJSON() ([]byte, error) {
	if p == nil {
		return []byte("null"), nil
	}
	out := make(map[string]json.RawMessage, len(p))
	for k, v := range p {
		if v == nil {
			out[k] = json.RawMessage("null")
			continue
		}
		// RawProviderOption holds raw JSON; pass it through directly.
		if raw, ok := v.(RawProviderOption); ok {
			if len(raw.Raw) == 0 {
				out[k] = json.RawMessage("null")
			} else {
				out[k] = raw.Raw
			}
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("provider: marshaling option %q: %w", k, err)
		}
		out[k] = b
	}
	return json.Marshal(out)
}

// UnmarshalJSON implements [json.Unmarshaler]. Every entry is wrapped as
// [RawProviderOption]; consumers call [ResolveOption] to recover typed values.
func (p *ProviderOptions) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*p = nil
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("provider: unmarshaling ProviderOptions: %w", err)
	}
	out := make(ProviderOptions, len(raw))
	for k, v := range raw {
		out[k] = RawProviderOption{Key: k, Raw: v}
	}
	*p = out
	return nil
}

// BuildProviderOptions constructs a [ProviderOptions] map from variadic typed
// values, using each value's ProviderKey() as the map key. When multiple
// values share the same key, the last value wins.
func BuildProviderOptions(opts ...ProviderOption) ProviderOptions {
	result := make(ProviderOptions, len(opts))
	for _, opt := range opts {
		result[opt.ProviderKey()] = opt
	}
	return result
}

// ResolveOption resolves a typed provider option from the map. It handles
// three cases:
//   - Key not present: returns (zero, false, nil)
//   - Value is type T: returns via direct type assertion (zero-cost path)
//   - Value is RawProviderOption: returns via json.Unmarshal into T
//   - Value is unexpected type: returns (zero, true, error)
//
// Resolution prefers the typed in-memory value over raw JSON when the map entry
// already has type T. RawProviderOption is only decoded when it is the stored
// value, which is the normal shape after crossing a JSON wire boundary.
func ResolveOption[T any](opts ProviderOptions, key string) (T, bool, error) {
	var zero T
	opt, ok := opts[key]
	if !ok {
		return zero, false, nil
	}
	switch v := opt.(type) {
	case T:
		return v, true, nil
	case RawProviderOption:
		var result T
		if err := json.Unmarshal(v.Raw, &result); err != nil {
			return zero, true, fmt.Errorf("provider: unmarshaling %q option: %w", key, err)
		}
		return result, true, nil
	default:
		return zero, true, fmt.Errorf("provider: unexpected type for %q: %T", key, opt)
	}
}
