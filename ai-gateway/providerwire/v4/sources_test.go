package v4

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourcesUnaryTitleAndIdentity(t *testing.T) {
	for _, tc := range []struct{ name, title, legacy, want string }{
		{"title", "Title", "", "Title"}, {"legacy", "", "Legacy", "Legacy"}, {"conflict", "Title", "Legacy", "Title"}, {"empty", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := provider.GenerateContentPart{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: "private-id", Title: tc.title, Text: tc.legacy, MediaType: "text/plain"}
			result := &provider.GenerateResult{Content: []provider.GenerateContentPart{part, part}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
			mapped, err := mapUnarySuccess(result, 4096)
			require.NoError(t, err)
			encoded, ok := encodeUnarySuccess(mapped, 4096)
			require.True(t, ok)
			var value struct {
				Content []map[string]any `json:"content"`
			}
			require.NoError(t, json.Unmarshal(encoded, &value))
			assert.Equal(t, tc.want, value.Content[0]["title"])
			assert.Equal(t, value.Content[0]["id"], value.Content[1]["id"])
			assert.NotContains(t, string(encoded), "private-id")
			assert.NotContains(t, value.Content[0], "filename")
		})
	}
}
