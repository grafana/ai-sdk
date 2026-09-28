package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_AsyncToolCallContinuation(t *testing.T) {
	for _, tc := range []struct {
		name, toolID, toolName string
		store                  bool
		async                  bool
		wantType               string
	}{
		{name: "function nonstored true", toolName: "lookup", async: true, wantType: "function_call"},
		{name: "function nonstored false", toolName: "lookup", wantType: "function_call"},
		{name: "function stored false remains inline", toolName: "lookup", store: true, wantType: "function_call"},
		{name: "custom nonstored true", toolID: toolIDCustom, toolName: "write_sql", async: true, wantType: "custom_tool_call"},
		{name: "custom stored remains reference", toolID: toolIDCustom, toolName: "write_sql", store: true, async: true, wantType: "item_reference"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := provider.ToolCallPart("call_1", tc.toolName, json.RawMessage(`{}`))
			part.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{
				ItemID: "item_1", Async: &tc.async, Namespace: "ns", Caller: &OpenAIToolCaller{Type: OpenAIToolCallerProgram, CallerID: "prog_1"},
			})
			result := provider.ToolResultPart("call_1", tc.toolName, &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "done"})
			tools := []provider.Tool{{Type: provider.ToolTypeFunction, Name: tc.toolName}}
			if tc.toolID != "" {
				tools[0] = provider.Tool{Type: provider.ToolTypeProvider, ID: tc.toolID, Name: tc.toolName}
			}
			body, _ := buildBody(t, "gpt-6-astra", provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(part), provider.NewToolMessage(result)},
				Tools:  tools, ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &tc.store}),
			})
			input := body["input"].([]any)
			require.NotEmpty(t, input)
			call := input[0].(map[string]any)
			assert.Equal(t, tc.wantType, call["type"])
			if tc.wantType == "item_reference" {
				assert.Equal(t, "item_1", call["id"])
			} else {
				assert.Equal(t, tc.async, call["async"])
				assert.Equal(t, "call_1", call["call_id"])
				if tc.toolID == "" {
					assert.Equal(t, "ns", call["namespace"])
					assert.Equal(t, "program", call["caller"].(map[string]any)["type"])
				}
			}
			assert.Equal(t, "call_1", input[1].(map[string]any)["call_id"])
		})
	}
}
