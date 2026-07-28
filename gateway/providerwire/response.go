package providerwire

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

// EncodeGenerateResult serializes a [provider.GenerateResult] for the wire.
// All fields round-trip losslessly: Content (every GenerateContentType),
// FinishReason (Unified + Raw), Usage, ProviderMetadata, Warnings, Request,
// and Response.
func EncodeGenerateResult(result *provider.GenerateResult) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("wire: nil GenerateResult")
	}
	b, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("wire: encoding GenerateResult: %w", err)
	}
	return b, nil
}

// DecodeGenerateResult deserializes a [provider.GenerateResult] from the wire.
func DecodeGenerateResult(data []byte) (*provider.GenerateResult, error) {
	var result provider.GenerateResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("wire: decoding GenerateResult: %w", err)
	}
	return &result, nil
}
