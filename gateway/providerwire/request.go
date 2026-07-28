package providerwire

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

// EncodeCallOptions serializes [provider.CallOptions] for the wire.
//
// All CallOptions fields round-trip losslessly: Prompt (with all message
// roles and content-part variants), Tools (function and provider), and every
// scalar/nullable knob. Typed ProviderOption values are serialized via their
// own JSON encoding; on decode they come back as [provider.RawProviderOption]
// (see [DecodeCallOptions]).
func EncodeCallOptions(opts provider.CallOptions) ([]byte, error) {
	b, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("wire: encoding CallOptions: %w", err)
	}
	return b, nil
}

// DecodeCallOptions deserializes a [provider.CallOptions] from the wire.
// ProviderOption values come back as [provider.RawProviderOption]; consumers
// reach typed views via [provider.ResolveOption].
func DecodeCallOptions(data []byte) (provider.CallOptions, error) {
	var opts provider.CallOptions
	if err := json.Unmarshal(data, &opts); err != nil {
		return provider.CallOptions{}, fmt.Errorf("wire: decoding CallOptions: %w", err)
	}
	return opts, nil
}
