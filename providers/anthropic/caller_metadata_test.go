package anthropic

import (
	"encoding/json"
	"fmt"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallerMetadata_ServerToolRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name      string
		block     string
		errorJSON string
	}{
		{name: "server call", block: `{"type":"server_tool_use","id":"call_1","name":"web_search","input":{}}`},
		{name: "search result", block: `{"type":"web_search_tool_result","tool_use_id":"call_1","content":[]}`},
		{name: "search error", block: `{"type":"web_search_tool_result","tool_use_id":"call_1","content":{"type":"web_search_tool_result_error","error_code":"invalid_tool_input"}}`, errorJSON: `{"type":"web_search_tool_result_error","errorCode":"invalid_tool_input"}`},
		{name: "fetch result", block: `{"type":"web_fetch_tool_result","tool_use_id":"call_1","content":{"type":"web_fetch_result","url":"https://example.test","retrieved_at":"2026-09-01T00:00:00Z","content":{"type":"document","title":"Document","source":{"type":"text","media_type":"text/plain","data":"hello"}}}}`},
		{name: "fetch error", block: `{"type":"web_fetch_tool_result","tool_use_id":"call_1","content":{"type":"web_fetch_tool_result_error","error_code":"invalid_input"}}`, errorJSON: `{"type":"web_fetch_tool_result_error","errorCode":"invalid_input"}`},
	} {
		for _, caller := range []string{`{"type":"direct"}`, `{"type":"code_execution_20260120","tool_id":"program_1"}`} {
			t.Run(tc.name+caller, func(t *testing.T) {
				block := tc.block[:len(tc.block)-1] + `,"caller":` + caller + `}`
				message := unmarshalMessage(t, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[`+block+`],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
				result, err := convertResponse(message, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
				require.NoError(t, err)
				require.NotEmpty(t, result.Content)
				part := result.Content[0]
				assert.Equal(t, "call_1", part.ToolCallID)
				if tc.errorJSON != "" {
					assert.True(t, part.IsError)
					assert.JSONEq(t, tc.errorJSON, string(part.Result))
				}
				require.Contains(t, part.ProviderMetadata, "anthropic")
				var expected map[string]any
				require.NoError(t, json.Unmarshal([]byte(caller), &expected))
				if id, ok := expected["tool_id"]; ok {
					delete(expected, "tool_id")
					expected["toolId"] = id
				}
				var metadata map[string]any
				require.NoError(t, json.Unmarshal(part.ProviderMetadata["anthropic"], &metadata))
				assert.Equal(t, expected, metadata["caller"])

				events := []sdk.BetaRawMessageStreamEventUnion{
					unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":`+block+`}`),
					unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
				}
				parts := collectParts(events)
				var found bool
				for _, streamed := range parts {
					if streamed.Type == provider.PartToolCall || streamed.Type == provider.PartToolResult {
						assert.Equal(t, part.ProviderMetadata, streamed.ProviderMetadata)
						assert.Equal(t, part.ToolCallID, streamed.ToolCallID)
						assert.Equal(t, part.ToolName, streamed.ToolName)
						assert.Equal(t, part.IsError, streamed.IsError)
						if streamed.Type == provider.PartToolResult {
							assert.JSONEq(t, string(part.Result), string(streamed.Result))
						}
						found = true
					}
				}
				require.True(t, found)

				continuation := provider.ContentPart{
					ToolCallID: part.ToolCallID, ToolName: part.ToolName, ProviderExecuted: true,
					ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: part.ProviderMetadata["anthropic"]}},
				}
				if part.Type == provider.ContentToolCall {
					continuation.Type, continuation.Input = provider.ContentPartTypeToolCall, part.Input
				} else {
					continuation.Type = provider.ContentPartTypeToolResult
					continuation.Output = &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: part.Result}
					if part.IsError {
						continuation.Output.Type = provider.ToolOutputErrorJSON
					}
				}
				params, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(continuation)}}, false)
				require.NoError(t, err)
				require.Len(t, params.Messages, 1)
				require.Len(t, params.Messages[0].Content, 1)
				encoded, err := json.Marshal(params.Messages[0].Content[0])
				require.NoError(t, err)
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(encoded, &fields))
				assert.JSONEq(t, caller, string(fields["caller"]), fmt.Sprintf("request: %s", encoded))
				if tc.errorJSON != "" {
					assert.JSONEq(t, block, string(encoded))
				}
			})
		}
	}
}
