package openaicompatible

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequest_AssistantHistoryToolInput(t *testing.T) {
	m := &model{modelID: "test-model"}
	for _, tc := range []struct {
		name, input, want string
	}{
		{"object", `{"city":"Berlin"}`, `{"city":"Berlin"}`},
		{"string object", `"  { \"city\": \"Berlin\" }\n"`, "  { \"city\": \"Berlin\" }\n"},
		{"string array", `"[1]"`, `[1]`},
		{"string scalar", `"42"`, `42`},
		{"string null", `"null"`, `null`},
		{"string containing JSON string", `"\"42\""`, `"42"`},
		{"malformed inner JSON", `"{\"city\":}"`, `"{\"city\":}"`},
		{"trailing JSON value", `"{} {}"`, `"{} {}"`},
		{"rejected inner array", `"[{\"__proto__\":false}]"`, `"[{\"__proto__\":false}]"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, body, _, err := m.prepareRequest(provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(
					provider.ToolCallPart("call_1", "weather", json.RawMessage(tc.input)),
				)},
			}, false)
			require.NoError(t, err)
			var request struct {
				Messages []struct {
					ToolCalls []struct {
						Function struct {
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"messages"`
			}
			require.NoError(t, json.Unmarshal(body, &request))
			require.Len(t, request.Messages, 1)
			require.Len(t, request.Messages[0].ToolCalls, 1)
			assert.Equal(t, tc.want, request.Messages[0].ToolCalls[0].Function.Arguments)
		})
	}
}
