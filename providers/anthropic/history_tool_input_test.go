package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_AssistantHistoryToolInput(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{"object", `{"city":"Berlin"}`, `{"city":"Berlin"}`},
		{"string object", `"  { \"city\": \"Berlin\" }\n"`, `{"city":"Berlin"}`},
		{"string array", `"[1]"`, `{"rawInvalidInput":"[1]"}`},
		{"string scalar", `"42"`, `{"rawInvalidInput":"42"}`},
		{"string null", `"null"`, `{"rawInvalidInput":"null"}`},
		{"malformed inner JSON", `"{\"city\":}"`, `{"rawInvalidInput":"{\"city\":}"}`},
		{"rejected inner object", `"{\"constructor\":{\"prototype\":false}}"`, `{"rawInvalidInput":"{\"constructor\":{\"prototype\":false}}"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(
					provider.ToolCallPart("call_1", "weather", json.RawMessage(tc.input)),
				)},
			}, false)
			require.NoError(t, err)
			require.Len(t, params.Messages, 1)
			require.Len(t, params.Messages[0].Content, 1)
			block := params.Messages[0].Content[0].OfToolUse
			require.NotNil(t, block)
			body, err := json.Marshal(block.Input)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(body))
		})
	}
}
