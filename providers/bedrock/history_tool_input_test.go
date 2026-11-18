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
		{"direct object retains representation", `{"id":9007199254740993,"label":"\ud800"}`, `{"id":9007199254740993,"label":"\ud800"}`},
		{"string object preserves numeric precision", `"{\"id\":9007199254740993}"`, `{"id":9007199254740993}`},
		{"string object uses Go Unicode decoding", `"{\"label\":\"\\ud800\\ud83d\\ude00\"}"`, `{"label":"�😀"}`},
		{"string object keeps the last duplicate member", `"{\"meta\":{\"city\":\"Paris\",\"city\":\"Berlin\"}}"`, `{"meta":{"city":"Berlin"}}`},
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
			body := marshalRequestBody(t, req)
			var messages []converseMessage
			require.NoError(t, json.Unmarshal(body["messages"], &messages))
			require.Len(t, messages, 1)
			require.Len(t, messages[0].Content, 1)
			block := messages[0].Content[0].ToolUse
			require.NotNil(t, block)
			assert.Equal(t, tc.want, string(block.Input))
		})
	}
}
