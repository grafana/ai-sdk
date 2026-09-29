package agentobservability

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamRecorder_SourceTimingWithoutCapture(t *testing.T) {
	recorder := newRecorderForStreamTest()
	source := &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "private-id", URL: "https://private.example", Title: "private-title", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"fileId":"secret"}`)}}
	recorder.Observe(provider.StreamPart{Type: provider.PartSource, Source: source})
	assert.False(t, recorder.FirstChunkAt().IsZero())
	generation := recorder.Generation()
	assert.Empty(t, generation.Output)
	encoded, err := json.Marshal(generation)
	require.NoError(t, err)
	for _, private := range []string{"private-id", "private.example", "private-title", "secret"} {
		assert.NotContains(t, string(encoded), private)
	}
	assert.Equal(t, "private-title", source.Title)
}
