package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSearchError_Continuation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output json.RawMessage
	}{
		{name: "object", output: json.RawMessage(`{"errorCode":"invalid_tool_input"}`)},
		{name: "JSON string", output: json.RawMessage(`"{\"errorCode\":\"invalid_tool_input\"}"`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := provider.ToolResultPart("search_1", "web_search", &provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: tc.output})
			part.ProviderExecuted = true
			part.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"caller":{"type":"code_execution_20260120","toolId":"program_1"}}`)}}
			params, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(part)}}, false)
			require.NoError(t, err)
			require.Len(t, params.Messages, 1)
			encoded, err := json.Marshal(params.Messages[0].Content)
			require.NoError(t, err)
			assert.JSONEq(t, `[{"type":"web_search_tool_result","tool_use_id":"search_1","content":{"type":"web_search_tool_result_error","error_code":"invalid_tool_input"},"caller":{"type":"code_execution_20260120","tool_id":"program_1"}}]`, string(encoded))
		})
	}
}
