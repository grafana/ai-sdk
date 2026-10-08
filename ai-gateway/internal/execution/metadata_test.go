package execution

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadata_NamespaceIsolation(t *testing.T) {
	original := provider.ProviderMetadata{
		"gateway": json.RawMessage(`{"execution":{"native":true},"empty":{},"null":null,"unknown":[1,false]}`),
		"native":  json.RawMessage(` {"opaque":false} `),
	}
	before := cloneMetadata(original)
	overview := &Overview{RequestedModelID: "alias", CanonicalModelID: "public", Attempts: []Attempt{{Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected}}}
	got := Metadata(overview, original, func(provider.ProviderMetadata) bool { return true })
	assert.Equal(t, before, original)
	assert.Equal(t, original["native"], got["native"])
	var namespace struct {
		Execution      Overview        `json:"execution"`
		NativeMetadata json.RawMessage `json:"nativeMetadata"`
	}
	require.NoError(t, json.Unmarshal(got["gateway"], &namespace))
	assert.Equal(t, *overview, namespace.Execution)
	assert.JSONEq(t, string(original["gateway"]), string(namespace.NativeMetadata))
	got["native"][2] = 'x'
	assert.Equal(t, before, original)
}

func TestMetadata_CompleteEnvelopeLimits(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "response", true: "SSE frame"}[streaming], func(t *testing.T) {
			original := provider.ProviderMetadata{"gateway": json.RawMessage(`{"opaque":"native namespace"}`)}
			overview := &Overview{RequestedModelID: "alias", CanonicalModelID: "public", Attempts: []Attempt{{Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected}}}
			encode := func(metadata provider.ProviderMetadata) []byte {
				document, err := json.Marshal(struct {
					Type     string                    `json:"type"`
					Text     string                    `json:"text"`
					Metadata provider.ProviderMetadata `json:"providerMetadata"`
				}{Type: "finish", Text: "successful answer", Metadata: metadata})
				require.NoError(t, err)
				if streaming {
					return append(append([]byte("data: "), document...), '\n', '\n')
				}
				return document
			}
			fitting := Metadata(overview, original, func(provider.ProviderMetadata) bool { return true })
			fullSize := len(encode(fitting))
			for _, limit := range []int{fullSize, fullSize - 1, len(encode(original))} {
				got := Metadata(overview, original, func(metadata provider.ProviderMetadata) bool { return len(encode(metadata)) <= limit })
				if limit == fullSize {
					assert.Equal(t, fitting, got)
				} else {
					assert.Equal(t, original, got)
				}
				assert.LessOrEqual(t, len(encode(got)), limit)
				assert.Contains(t, string(encode(got)), "successful answer")
			}
		})
	}
}

func TestMetadata_MalformedOptionalEnrichment(t *testing.T) {
	original := provider.ProviderMetadata{"native": json.RawMessage(`{"opaque":true}`)}
	overview := &Overview{Attempts: []Attempt{{Error: &Failure{Code: json.RawMessage(`invalid`)}}}}
	called := false
	got := Metadata(overview, original, func(provider.ProviderMetadata) bool { called = true; return true })
	assert.Equal(t, original, got)
	assert.False(t, called)
	assert.Equal(t, original, Metadata(nil, original, func(provider.ProviderMetadata) bool { t.Error("unexpected fit check"); return true }))
}

func TestMetadata_PrimaryValidationPreserved(t *testing.T) {
	original := provider.ProviderMetadata{"gateway": json.RawMessage(`true`)}
	overview := &Overview{RequestedModelID: "alias", CanonicalModelID: "public", Attempts: []Attempt{{Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected}}}
	checks := 0
	got := Metadata(overview, original, func(metadata provider.ProviderMetadata) bool {
		checks++
		var namespace map[string]json.RawMessage
		return json.Unmarshal(metadata["gateway"], &namespace) == nil && namespace != nil
	})
	assert.Equal(t, original, got)
	assert.Equal(t, 1, checks)
}

func cloneMetadata(metadata provider.ProviderMetadata) provider.ProviderMetadata {
	copy := make(provider.ProviderMetadata, len(metadata))
	for key, value := range metadata {
		copy[key] = append(json.RawMessage(nil), value...)
	}
	return copy
}
