package execution

import (
	"bytes"
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
)

type namespace struct {
	Execution      *Overview       `json:"execution"`
	NativeMetadata json.RawMessage `json:"nativeMetadata,omitempty"`
}

// Metadata adds optional execution attribution without changing native inputs.
// fits must check the caller's complete response or SSE frame, including primary
// validation and its existing limit. Encoding or fit failure preserves the
// original opaque metadata, including a native gateway namespace that cannot
// be relocated within that limit.
func Metadata(overview *Overview, original provider.ProviderMetadata, fits func(provider.ProviderMetadata) bool) provider.ProviderMetadata {
	if overview == nil {
		return original
	}
	encoded, err := json.Marshal(namespace{Execution: overview, NativeMetadata: original["gateway"]})
	if err != nil || !fits(original) {
		return original
	}
	metadata := make(provider.ProviderMetadata, len(original)+1)
	for key, value := range original {
		metadata[key] = bytes.Clone(value)
	}
	metadata["gateway"] = encoded
	if !fits(metadata) {
		return original
	}
	return metadata
}
