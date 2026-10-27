package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_StoredReasoningText(t *testing.T) {
	for _, tc := range []struct {
		name      string
		model     string
		reasoning bool
		duplicate bool
		wantRefs  int
	}{
		{"pruned reasoning", "gpt-6", false, false, 0},
		{"preceding reasoning", "gpt-6", true, false, 1},
		{"repeated text ID", "gpt-6", true, true, 1},
		{"nonreasoning repeated text ID", "gpt-4o", false, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := []provider.ContentPart{}
			if tc.reasoning {
				parts = append(parts, provider.ContentPart{Type: provider.ContentPartTypeReasoning, Text: "summary", ProviderOptions: provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"itemId":"rs_1"}`)}}})
			}
			text := provider.ContentPart{Type: provider.ContentPartTypeText, Text: "answer", ProviderOptions: provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"itemId":"msg_1"}`)}}}
			parts = append(parts, text)
			if tc.duplicate {
				parts = append(parts, text)
			}
			params, _, _, err := buildParams(tc.model, provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(parts...)}})
			require.NoError(t, err)
			encoded, err := json.Marshal(params)
			require.NoError(t, err)
			var request struct {
				Input []map[string]json.RawMessage `json:"input"`
			}
			require.NoError(t, json.Unmarshal(encoded, &request))
			references := 0
			for _, item := range request.Input {
				if string(item["type"]) == `"item_reference"` && string(item["id"]) == `"msg_1"` {
					references++
				}
			}
			assert.Equal(t, tc.wantRefs, references)
			if tc.wantRefs == 0 {
				assert.Contains(t, string(encoded), "answer")
			}
		})
	}
}
