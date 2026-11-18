package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequest_AssistantHistoryToolInput(t *testing.T) {
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
			req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(
					provider.ToolCallPart("call_1", "weather", json.RawMessage(tc.input)),
				)},
				Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)}},
			})
			require.Len(t, req.Messages, 1)
			require.Len(t, req.Messages[0].Content, 1)
			block := req.Messages[0].Content[0].ToolUse
			require.NotNil(t, block)
			assert.JSONEq(t, tc.want, string(block.Input))
		})
	}
}
