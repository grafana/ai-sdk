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
	got := Metadata(overview, original)
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

func TestMetadata_Unavailable(t *testing.T) {
	for _, tc := range []struct {
		name     string
		overview *Overview
		native   json.RawMessage
	}{
		{name: "absent overview", native: json.RawMessage(`{"opaque":true}`)},
		{name: "malformed overview", overview: &Overview{Attempts: []Attempt{{Error: &Failure{Code: json.RawMessage(`invalid`)}}}}, native: json.RawMessage(`{"opaque":true}`)},
		{name: "malformed native namespace", overview: &Overview{}, native: json.RawMessage(`invalid`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := provider.ProviderMetadata{"gateway": tc.native}
			before := cloneMetadata(original)
			assert.Nil(t, Metadata(tc.overview, original))
			assert.Equal(t, before, original)
		})
	}
}

func cloneMetadata(metadata provider.ProviderMetadata) provider.ProviderMetadata {
	copy := make(provider.ProviderMetadata, len(metadata))
	for key, value := range metadata {
		copy[key] = append(json.RawMessage(nil), value...)
	}
	return copy
}
