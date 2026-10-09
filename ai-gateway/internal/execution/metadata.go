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

// Metadata builds optional execution attribution without changing native inputs.
// It returns nil when attribution is absent or cannot be encoded. The caller
// must validate the original output and accept the candidate only when its
// complete response or SSE frame remains valid and within existing limits.
func Metadata(overview *Overview, original provider.ProviderMetadata) provider.ProviderMetadata {
	if overview == nil {
		return nil
	}
	encoded, err := json.Marshal(namespace{Execution: overview, NativeMetadata: original["gateway"]})
	if err != nil {
		return nil
	}
	metadata := make(provider.ProviderMetadata, len(original)+1)
	for key, value := range original {
		metadata[key] = bytes.Clone(value)
	}
	metadata["gateway"] = encoded
	return metadata
}
