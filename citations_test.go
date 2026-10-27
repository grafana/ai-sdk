package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamText_Citations(t *testing.T) {
	for _, citations := range [][]provider.Citation{nil, {}, {{Source: provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "u", URL: "https://example.com", ProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`{}`)}}, StartIndex: new(0), CitedText: new("")}}} {
		t.Run(string(mustMarshalCitations(t, citations)), func(t *testing.T) {
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				stream := make(chan provider.StreamPart, 4)
				stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "t"}
				stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: "answer"}
				stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t", Citations: citations}
				stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
				close(stream)
				return &provider.StreamResult{Stream: stream}, nil
			}}
			result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("test")))
			for range result.FullStream() {
			}
			content := result.Content()
			require.Len(t, content, 1)
			text, ok := content[0].(TextContent)
			require.True(t, ok)
			assert.Equal(t, citations, text.Citations)
			if len(citations) > 0 {
				*text.Citations[0].StartIndex = 5
				text.Citations[0].Source.ProviderMetadata["test"][0] = 'x'
				assert.Zero(t, *citations[0].StartIndex)
				assert.JSONEq(t, `{}`, string(citations[0].Source.ProviderMetadata["test"]))
			}
		})
	}
}

func mustMarshalCitations(t *testing.T, citations []provider.Citation) []byte {
	t.Helper()
	encoded, err := json.Marshal(citations)
	require.NoError(t, err)
	return encoded
}
