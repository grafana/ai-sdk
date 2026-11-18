package openai

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
			body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(
					provider.ToolCallPart("call_1", "weather", json.RawMessage(tc.input)),
				)},
			})
			call := findInput(body, "function_call")
			require.NotNil(t, call)
			assert.Equal(t, tc.want, call["arguments"])
		})
	}
}
