package execution

import (
	"bytes"
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
)

type namespace struct {
	Execution      *Overview       `json:"execution"`
	Routing        *Routing        `json:"routing,omitempty"`
	NativeMetadata json.RawMessage `json:"nativeMetadata,omitempty"`
}

// Metadata builds optional execution attribution without changing native inputs.
// It places the overview under gateway.execution and its routing summary under
// gateway.routing, and always relocates a native gateway value under
// gateway.nativeMetadata. It returns nil when the overview is absent or cannot
// be encoded. The caller validates the original output first and encodes the
// enriched output once.
func Metadata(overview *Overview, original provider.ProviderMetadata) provider.ProviderMetadata {
	if overview == nil {
		return nil
	}
	encoded, err := json.Marshal(namespace{Execution: overview, Routing: projectRouting(overview), NativeMetadata: original["gateway"]})
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
