package providerwirev4

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type providerOptionsDTO map[string]json.RawMessage

func encodeProviderOptions(options provider.ProviderOptions) (providerOptionsDTO, error) {
	if len(options) == 0 {
		return nil, nil
	}
	encoded := make(providerOptionsDTO, len(options))
	for key, option := range options {
		var value json.RawMessage
		if raw, ok := option.(provider.RawProviderOption); ok {
			value = append(json.RawMessage(nil), raw.Raw...)
		} else {
			data, err := json.Marshal(option)
			if err != nil {
				return nil, fmt.Errorf("providerwirev4: encoding provider option %q: %w", key, err)
			}
			value = data
		}
		if _, err := decodeObject(value, fmt.Sprintf("provider option %q", key)); err != nil {
			return nil, err
		}
		encoded[key] = value
	}
	return encoded, nil
}

func encodeNestedProviderOptions(options provider.ProviderOptions, context string) (providerOptionsDTO, error) {
	if _, exists := options["gateway"]; exists {
		return nil, fmt.Errorf("providerwirev4: %s must not contain reserved provider option %q", context, "gateway")
	}
	return encodeProviderOptions(options)
}

func decodeProviderOptions(options providerOptionsDTO) (provider.ProviderOptions, error) {
	if len(options) == 0 {
		return nil, nil
	}
	decoded := make(provider.ProviderOptions, len(options))
	for key, value := range options {
		if _, err := decodeObject(value, fmt.Sprintf("provider option %q", key)); err != nil {
			return nil, err
		}
		decoded[key] = provider.RawProviderOption{Key: key, Raw: append(json.RawMessage(nil), value...)}
	}
	return decoded, nil
}

func decodeNestedProviderOptions(options providerOptionsDTO, context string) (provider.ProviderOptions, error) {
	if _, exists := options["gateway"]; exists {
		return nil, fmt.Errorf("providerwirev4: %s must not contain reserved provider option %q", context, "gateway")
	}
	return decodeProviderOptions(options)
}
