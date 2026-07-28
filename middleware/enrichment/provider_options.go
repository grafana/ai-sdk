package enrichment

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

func providerOptionsOutputEnabled(opts ProviderOptionsConfig) bool {
	return opts.ProviderKey != ""
}

func providerOptionsSelection(opts ProviderOptionsConfig) map[string]struct{} {
	if len(opts.Map) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(opts.Map))
	for key := range opts.Map {
		if key != "" {
			set[key] = struct{}{}
		}
	}
	return set
}

func applyProviderOptions(params provider.CallOptions, values []Value, opts ProviderOptionsConfig) (provider.CallOptions, error) {
	if !providerOptionsOutputEnabled(opts) || len(values) == 0 {
		return params, nil
	}

	enrichmentFields, err := providerOptionFields(values, opts.Map)
	if err != nil {
		return params, err
	}
	if len(enrichmentFields) == 0 {
		return params, nil
	}

	providerOptions := cloneProviderOptions(params.ProviderOptions)
	if providerOptions == nil {
		providerOptions = make(provider.ProviderOptions)
	}

	merged, changed, err := mergeProviderOption(providerOptions[opts.ProviderKey], opts, enrichmentFields)
	if err != nil {
		return params, err
	}
	if !changed {
		return params, nil
	}

	providerOptions[opts.ProviderKey] = provider.RawProviderOption{Key: opts.ProviderKey, Raw: merged}
	params.ProviderOptions = providerOptions
	return params, nil
}

func providerOptionFields(values []Value, fieldMap map[string]string) (map[string]json.RawMessage, error) {
	fields := make(map[string]json.RawMessage, len(values))
	for _, value := range values {
		field := value.Key
		if mapped, ok := fieldMap[value.Key]; ok {
			field = mapped
		}
		field = normalizedKey(field)
		if field == "" {
			continue
		}
		raw, err := json.Marshal(value.Value)
		if err != nil {
			return nil, fmt.Errorf("enrichment: marshaling provider option field %q: %w", field, err)
		}
		fields[field] = raw
	}
	return fields, nil
}

func mergeProviderOption(existing provider.ProviderOption, opts ProviderOptionsConfig, fields map[string]json.RawMessage) (json.RawMessage, bool, error) {
	policy := canonicalConflictPolicy(opts.Conflict)
	if existing == nil {
		return buildProviderOptionObject(nil, opts.ObjectKey, fields, policy)
	}

	raw, err := marshalProviderOption(existing)
	if err != nil {
		return nil, false, err
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		if policy == ConflictError {
			return nil, false, fmt.Errorf("enrichment: provider option %q is not an object: %w", opts.ProviderKey, err)
		}
		if policy == ConflictCallerWins {
			return nil, false, nil
		}
		return buildProviderOptionObject(nil, opts.ObjectKey, fields, policy)
	}
	if obj == nil {
		if policy == ConflictError {
			return nil, false, fmt.Errorf("enrichment: provider option %q is not an object", opts.ProviderKey)
		}
		if policy == ConflictCallerWins {
			return nil, false, nil
		}
		return buildProviderOptionObject(nil, opts.ObjectKey, fields, policy)
	}

	return buildProviderOptionObject(obj, opts.ObjectKey, fields, policy)
}

func marshalProviderOption(opt provider.ProviderOption) (json.RawMessage, error) {
	if raw, ok := opt.(provider.RawProviderOption); ok {
		if len(raw.Raw) == 0 {
			return json.RawMessage("null"), nil
		}
		return cloneRaw(raw.Raw), nil
	}
	data, err := json.Marshal(opt)
	if err != nil {
		return nil, fmt.Errorf("enrichment: marshaling provider option %q: %w", opt.ProviderKey(), err)
	}
	return data, nil
}

func buildProviderOptionObject(existing map[string]json.RawMessage, objectKey string, fields map[string]json.RawMessage, policy ConflictPolicy) (json.RawMessage, bool, error) {
	obj := cloneRawMap(existing)
	if obj == nil {
		obj = make(map[string]json.RawMessage)
	}

	changed := false
	if objectKey == "" {
		var err error
		changed, err = mergeFields(obj, fields, policy)
		if err != nil {
			return nil, false, err
		}
	} else {
		nested, nestedChanged, err := nestedObject(obj, objectKey, policy)
		if err != nil {
			return nil, false, err
		}
		if nested == nil {
			return nil, false, nil
		}
		fieldChanged, err := mergeFields(nested, fields, policy)
		if err != nil {
			return nil, false, err
		}
		if nestedChanged || fieldChanged {
			rawNested, err := json.Marshal(nested)
			if err != nil {
				return nil, false, fmt.Errorf("enrichment: marshaling nested provider option %q: %w", objectKey, err)
			}
			obj[objectKey] = rawNested
			changed = true
		}
	}

	if !changed {
		return nil, false, nil
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, false, fmt.Errorf("enrichment: marshaling provider option object: %w", err)
	}
	return raw, true, nil
}

func nestedObject(obj map[string]json.RawMessage, objectKey string, policy ConflictPolicy) (map[string]json.RawMessage, bool, error) {
	raw, ok := obj[objectKey]
	if !ok {
		return make(map[string]json.RawMessage), true, nil
	}

	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil || nested == nil {
		if policy == ConflictError {
			if err != nil {
				return nil, false, fmt.Errorf("enrichment: provider option field %q is not an object: %w", objectKey, err)
			}
			return nil, false, fmt.Errorf("enrichment: provider option field %q is not an object", objectKey)
		}
		if policy == ConflictCallerWins {
			return nil, false, nil
		}
		return make(map[string]json.RawMessage), true, nil
	}
	return cloneRawMap(nested), false, nil
}

func mergeFields(target map[string]json.RawMessage, fields map[string]json.RawMessage, policy ConflictPolicy) (bool, error) {
	changed := false
	for key, value := range fields {
		if _, exists := target[key]; exists {
			switch policy {
			case ConflictEnrichmentWins:
				target[key] = cloneRaw(value)
				changed = true
			case ConflictError:
				return false, fmt.Errorf("enrichment: provider option field %q already exists", key)
			default:
				continue
			}
			continue
		}
		target[key] = cloneRaw(value)
		changed = true
	}
	return changed, nil
}

func cloneProviderOptions(opts provider.ProviderOptions) provider.ProviderOptions {
	if len(opts) == 0 {
		return nil
	}
	out := make(provider.ProviderOptions, len(opts))
	for key, value := range opts {
		out[key] = value
	}
	return out
}

func cloneRawMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(in))
	for key, value := range in {
		out[key] = cloneRaw(value)
	}
	return out
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	out := make(json.RawMessage, len(raw))
	copy(out, raw)
	return out
}
