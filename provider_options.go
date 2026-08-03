package aisdk

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

func mergeStepProviderOptions(base, override provider.ProviderOptions) (provider.ProviderOptions, error) {
	if base == nil {
		return override, nil
	}
	if override == nil {
		return base, nil
	}

	baseObject, err := providerOptionsObject(base)
	if err != nil {
		return nil, fmt.Errorf("marshaling base options: %w", err)
	}
	overrideObject, err := providerOptionsObject(override)
	if err != nil {
		return nil, fmt.Errorf("marshaling override options: %w", err)
	}
	merged := mergeJSONObject(baseObject, overrideObject)

	result := make(provider.ProviderOptions, len(merged))
	for key, value := range merged {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshaling merged option %q: %w", key, err)
		}
		result[key] = provider.RawProviderOption{Key: key, Raw: raw}
	}
	return result, nil
}

func providerOptionsObject(options provider.ProviderOptions) (map[string]any, error) {
	data, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	return object, nil
}

func mergeJSONObject(base, override map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(override))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range override {
		if key == "__proto__" || key == "constructor" || key == "prototype" {
			continue
		}
		baseObject, baseOK := result[key].(map[string]any)
		overrideObject, overrideOK := value.(map[string]any)
		if baseOK && overrideOK {
			result[key] = mergeJSONObject(baseObject, overrideObject)
			continue
		}
		result[key] = value
	}
	return result
}
