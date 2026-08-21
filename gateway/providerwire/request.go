package providerwire

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

// EncodeCallOptions maps [provider.CallOptions] to the tolerant legacy request
// representation and serializes it.
func EncodeCallOptions(options provider.CallOptions) ([]byte, error) {
	legacy, err := legacyCallOptionsFromProvider(options)
	if err != nil {
		return nil, fmt.Errorf("wire: encoding CallOptions: %w", err)
	}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		return nil, fmt.Errorf("wire: encoding CallOptions: %w", err)
	}
	return encoded, nil
}

// DecodeCallOptions decodes the tolerant legacy request representation and
// maps it to [provider.CallOptions].
func DecodeCallOptions(data []byte) (provider.CallOptions, error) {
	var legacy legacyCallOptions
	if err := json.Unmarshal(data, &legacy); err != nil {
		return provider.CallOptions{}, fmt.Errorf("wire: decoding CallOptions: %w", err)
	}
	options, err := legacy.toProvider()
	if err != nil {
		return provider.CallOptions{}, fmt.Errorf("wire: decoding CallOptions: %w", err)
	}
	return options, nil
}
