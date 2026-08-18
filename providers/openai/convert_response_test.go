package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeResponse unmarshals a Responses JSON body into the SDK type.
func decodeResponse(t *testing.T, body string) *responses.Response {
	t.Helper()
	var r responses.Response
	require.NoError(t, json.Unmarshal([]byte(body), &r))
	return &r
}

func mustConvertResponse(t *testing.T, resp *responses.Response, br buildResult) *provider.GenerateResult {
	t.Helper()
	result, err := convertResponse(resp, br, seqIDGen(), "openai")
	require.NoError(t, err)
	return result
}

func seqIDGen() func() string {
	n := 0
	return func() string {
		n++
		return "src_" + string(rune('0'+n))
	}
}

type mcpCallFieldPresenceCase struct {
	name     string
	fields   string
	approval bool
	want     map[string]any
}

func mcpCallFieldPresenceCases() []mcpCallFieldPresenceCase {
	base := func() map[string]any {
		return map[string]any{
			"type":        "call",
			"serverLabel": "srv",
			"name":        "do_thing",
			"arguments":   "{}",
		}
	}
	with := func(fields map[string]any) map[string]any {
		result := base()
		for key, value := range fields {
			result[key] = value
		}
		return result
	}
	return []mcpCallFieldPresenceCase{
		{name: "fields absent", want: base()},
		{name: "fields null", fields: `,"output":null,"error":null`, approval: true, want: base()},
		{name: "output only", fields: `,"output":"ok","error":null`, want: with(map[string]any{"output": "ok"})},
		{name: "string error only", fields: `,"output":null,"error":"failed"`, approval: true, want: with(map[string]any{"error": "failed"})},
		{name: "object error only", fields: `,"output":null,"error":{"type":"tool_error","code":500,"message":"failed"}`, approval: true, want: with(map[string]any{"error": map[string]any{"type": "tool_error", "code": float64(500), "message": "failed"}})},
		{name: "both fields", fields: `,"output":"partial","error":"failed"`, want: with(map[string]any{"output": "partial", "error": "failed"})},
	}
}

func TestConvertResponse_TextAndURLCitation(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[
			{"type":"output_text","text":"see here","annotations":[
				{"type":"url_citation","url":"https://x.com","title":"X","start_index":0,"end_index":3}
			]}
		]}],
		"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 2)
	assert.Equal(t, provider.ContentText, res.Content[0].Type)
	assert.Equal(t, "see here", res.Content[0].Text)
	assert.Equal(t, provider.ContentSource, res.Content[1].Type)
	assert.Equal(t, provider.SourceTypeURL, res.Content[1].SourceType)
	assert.Equal(t, "https://x.com", res.Content[1].URL)
	assert.Equal(t, "X", res.Content[1].Text)
	assert.Equal(t, provider.FinishReasonStop, res.FinishReason.Unified)
}

func TestConvertResponse_TextPhaseAndFunctionNamespaceMetadata(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[
			{"type":"message","id":"msg_1","role":"assistant","phase":"commentary","status":"completed","content":[{"type":"output_text","text":"draft","annotations":[]}]},
			{"type":"function_call","id":"fc_1","call_id":"call_1","name":"getWeather","namespace":"weather_ns","arguments":"{}","status":"completed"}
		],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 2)

	var textMeta map[string]any
	require.NoError(t, json.Unmarshal(res.Content[0].ProviderMetadata["openai"], &textMeta))
	assert.Equal(t, "msg_1", textMeta["itemId"])
	assert.Equal(t, "commentary", textMeta["phase"])

	var toolMeta map[string]any
	require.NoError(t, json.Unmarshal(res.Content[1].ProviderMetadata["openai"], &toolMeta))
	assert.Equal(t, "fc_1", toolMeta["itemId"])
	assert.Equal(t, "weather_ns", toolMeta["namespace"])
}

func TestConvertResponse_ProviderExecutedWebSearch(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"go"}}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 2)
	assert.Equal(t, provider.ContentToolCall, res.Content[0].Type)
	assert.True(t, res.Content[0].ProviderExecuted)
	assert.Equal(t, "web_search", res.Content[0].ToolName)
	assert.Equal(t, provider.ContentToolResult, res.Content[1].Type)
}

func TestConvertResponse_FileSearchResultFields(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"file_search_call","id":"fs_1","status":"completed","queries":["q"],"results":[{
			"attributes":{"author":"Jane Smith","pages":5,"published":true},
			"file_id":"file_1","filename":"guide.pdf","score":0.75,"text":"result text"
		}]}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 2)
	assert.JSONEq(t, `{
		"queries":["q"],
		"results":[{
			"attributes":{"author":"Jane Smith","pages":5,"published":true},
			"fileId":"file_1",
			"filename":"guide.pdf",
			"score":0.75,
			"text":"result text"
		}]
	}`, string(res.Content[1].Result))
	assert.NotContains(t, string(res.Content[1].Result), "file_id")
}

func TestConvertResponse_WebSearchPreviewUsesCustomToolName(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"go"}}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{webSearchToolName: "search"})
	require.Len(t, res.Content, 2)
	assert.Equal(t, "search", res.Content[0].ToolName)
	assert.Equal(t, "search", res.Content[1].ToolName)
}

func TestConvertResponse_ProviderExecutedToolNameMapping(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[
			{"type":"file_search_call","id":"fs_1","status":"completed","queries":["q"],"results":[]},
			{"type":"code_interpreter_call","id":"ci_1","status":"completed","container_id":"ctr_1","code":"print(1)","outputs":[]},
			{"type":"image_generation_call","id":"ig_1","status":"completed","result":"BASE64DATA"}
		],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	mapping := newToolNameMapping([]provider.Tool{
		{Type: provider.ToolTypeProvider, ID: toolIDFileSearch, Name: "docs"},
		{Type: provider.ToolTypeProvider, ID: toolIDCodeInterpreter, Name: "python"},
		{Type: provider.ToolTypeProvider, ID: toolIDImageGeneration, Name: "draw"},
	})
	res := mustConvertResponse(t, resp, buildResult{toolNameMapping: mapping})
	require.Len(t, res.Content, 6)
	assert.Equal(t, "docs", res.Content[0].ToolName)
	assert.Equal(t, "docs", res.Content[1].ToolName)
	assert.Equal(t, "python", res.Content[2].ToolName)
	assert.Equal(t, "python", res.Content[3].ToolName)
	assert.Equal(t, "draw", res.Content[4].ToolName)
	assert.Equal(t, "draw", res.Content[5].ToolName)
}

func TestConvertResponse_ComputerCall(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"computer-preview","object":"response","status":"completed",
		"output":[{
			"type":"computer_call","id":"item_1","call_id":"call_1","status":"completed",
			"actions":[
				{"type":"click","button":"left","x":1,"y":2,"keys":["SHIFT"]},
				{"type":"double_click","x":3,"y":4},
				{"type":"drag","path":[{"x":1,"y":2},{"x":3,"y":4}]},
				{"type":"keypress","keys":["CTRL","L"]},
				{"type":"move","x":5,"y":6},
				{"type":"screenshot"},
				{"type":"scroll","x":7,"y":8,"scroll_x":9,"scroll_y":10},
				{"type":"type","text":"hello"},
				{"type":"wait"}
			],
			"pending_safety_checks":[{"id":"safe_1","code":"policy","message":"confirm"}]
		}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	mapping := newToolNameMapping([]provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDComputer, Name: "browser"}})
	res := mustConvertResponse(t, resp, buildResult{hasComputerTool: true, toolNameMapping: mapping})
	require.Len(t, res.Content, 1)
	call := res.Content[0]
	assert.Equal(t, provider.ContentToolCall, call.Type)
	assert.Equal(t, "call_1", call.ToolCallID)
	assert.Equal(t, "browser", call.ToolName)
	assert.False(t, call.ProviderExecuted)
	assert.JSONEq(t, `{
		"actions":[
			{"type":"click","button":"left","x":1,"y":2,"keys":["SHIFT"]},
			{"type":"double_click","x":3,"y":4},
			{"type":"drag","path":[{"x":1,"y":2},{"x":3,"y":4}]},
			{"type":"keypress","keys":["CTRL","L"]},
			{"type":"move","x":5,"y":6},
			{"type":"screenshot"},
			{"type":"scroll","x":7,"y":8,"scrollX":9,"scrollY":10},
			{"type":"type","text":"hello"},
			{"type":"wait"}
		],
		"pendingSafetyChecks":[{"id":"safe_1","code":"policy","message":"confirm"}],
		"status":"completed"
	}`, string(call.Input))
	var metadata map[string]any
	require.NoError(t, json.Unmarshal(call.ProviderMetadata["openai"], &metadata))
	assert.Equal(t, "item_1", metadata["itemId"])
	assert.Equal(t, provider.FinishReasonToolCalls, res.FinishReason.Unified)
}

func TestConvertResponse_ComputerCallPrefersExplicitActions(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"computer-preview","object":"response","status":"completed",
		"output":[{
			"type":"computer_call","id":"item_1","call_id":"call_1","status":"completed",
			"actions":[],"action":{"type":"screenshot"},"pending_safety_checks":[]
		}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	mapping := newToolNameMapping([]provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDComputer, Name: "browser"}})
	res := mustConvertResponse(t, resp, buildResult{hasComputerTool: true, toolNameMapping: mapping})
	require.Len(t, res.Content, 1)
	assert.JSONEq(t, `{"actions":[],"pendingSafetyChecks":[],"status":"completed"}`, string(res.Content[0].Input))
}

func TestConvertResponse_ComputerCallFallsBackFromNullActions(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"computer-preview","object":"response","status":"completed",
		"output":[{
			"type":"computer_call","id":"item_1","call_id":"call_1","status":"completed",
			"actions":null,"action":{"type":"screenshot"},"pending_safety_checks":[]
		}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	mapping := newToolNameMapping([]provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDComputer, Name: "browser"}})
	res := mustConvertResponse(t, resp, buildResult{hasComputerTool: true, toolNameMapping: mapping})
	require.Len(t, res.Content, 1)
	assert.JSONEq(t, `{"actions":[{"type":"screenshot"}],"pendingSafetyChecks":[],"status":"completed"}`, string(res.Content[0].Input))
}

func TestConvertResponse_LegacyComputerCall(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"computer-preview","object":"response","status":"completed",
		"output":[{"type":"computer_call","id":"item_1","call_id":null,"status":"completed","action":{"type":"screenshot"},"pending_safety_checks":[]}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)
	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 2)
	assert.Equal(t, "computer_use", res.Content[0].ToolName)
	assert.True(t, res.Content[0].ProviderExecuted)
	assert.Equal(t, provider.ContentToolResult, res.Content[1].Type)
	assert.Equal(t, provider.FinishReasonStop, res.FinishReason.Unified)
}

func TestMCPApprovalRequestID(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "absent", raw: `{"type":"mcp_approval_request","id":"item_1","name":"do_thing","server_label":"srv","arguments":"{}"}`, want: "item_1"},
		{name: "null", raw: `{"type":"mcp_approval_request","id":"item_1","approval_request_id":null,"name":"do_thing","server_label":"srv","arguments":"{}"}`, want: "item_1"},
		{name: "empty", raw: `{"type":"mcp_approval_request","id":"item_1","approval_request_id":"","name":"do_thing","server_label":"srv","arguments":"{}"}`, want: ""},
		{name: "value", raw: `{"type":"mcp_approval_request","id":"item_1","approval_request_id":"appr_1","name":"do_thing","server_label":"srv","arguments":"{}"}`, want: "appr_1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var item responses.ResponseOutputItemMcpApprovalRequest
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &item))
			assert.Equal(t, tc.want, mcpApprovalRequestID(item))
		})
	}
}

func TestConvertResponse_MCPApprovalRequest(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"mcp_approval_request","id":"item_1","approval_request_id":"appr_1","name":"do_thing","server_label":"srv","arguments":"{}"}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 2)
	assert.Equal(t, provider.ContentToolCall, res.Content[0].Type)
	assert.True(t, res.Content[0].ProviderExecuted)
	require.NotNil(t, res.Content[0].Dynamic)
	assert.True(t, *res.Content[0].Dynamic)
	assert.Equal(t, provider.ContentToolApprovalRequest, res.Content[1].Type)
	assert.Equal(t, "appr_1", res.Content[1].ApprovalID)
	assert.Equal(t, res.Content[0].ToolCallID, res.Content[1].ToolCallID)
}

func TestConvertResponse_MCPCallUsesApprovalToolCallID(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"mcp_call","id":"mcp_1","approval_request_id":"appr_1","name":"do_thing","server_label":"srv","arguments":"{}","output":"ok"}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{
		approvalRequestToolCallIDs: map[string]string{"appr_1": "dummy_call_1"},
	})
	require.Len(t, res.Content, 2)
	assert.Equal(t, "dummy_call_1", res.Content[0].ToolCallID)
	assert.Equal(t, "dummy_call_1", res.Content[1].ToolCallID)
}

func TestConvertResponse_MCPCallUsesEmptyApprovalRequestID(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"mcp_call","id":"mcp_1","approval_request_id":"","name":"do_thing","server_label":"srv","arguments":"{}","output":"ok"}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{
		approvalRequestToolCallIDs: map[string]string{"": "dummy_call_1"},
	})
	require.Len(t, res.Content, 2)
	assert.Equal(t, "dummy_call_1", res.Content[0].ToolCallID)
	assert.Equal(t, "dummy_call_1", res.Content[1].ToolCallID)
}

func TestConvertResponse_MCPCallResultFieldPresence(t *testing.T) {
	for _, tc := range mcpCallFieldPresenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			approvalRequest := ""
			toolCallID := "mcp_1"
			br := buildResult{}
			if tc.approval {
				approvalRequest = `,"approval_request_id":"appr_1"`
				toolCallID = "dummy_call_1"
				br.approvalRequestToolCallIDs = map[string]string{"appr_1": toolCallID}
			}
			resp := decodeResponse(t, `{
				"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
				"output":[{"type":"mcp_call","id":"mcp_1","name":"do_thing","server_label":"srv","arguments":"{}"`+approvalRequest+tc.fields+`}],
				"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
			}`)

			res := mustConvertResponse(t, resp, br)
			require.Len(t, res.Content, 2)
			assert.Nil(t, res.Content[0].ProviderMetadata)
			assert.Equal(t, toolCallID, res.Content[1].ToolCallID)
			assert.JSONEq(t, `{"itemId":"mcp_1"}`, string(res.Content[1].ProviderMetadata["openai"]))
			var result map[string]any
			require.NoError(t, json.Unmarshal(res.Content[1].Result, &result))
			assert.Equal(t, tc.want, result)
		})
	}
}

func TestConvertResponse_MCPCallResultConversionError(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"mcp_call","id":"mcp_1","name":"do_thing","server_label":"srv","arguments":"{}","error":[]}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	result, err := convertResponse(resp, buildResult{}, seqIDGen(), "openai")
	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorContains(t, err, "openai: decoding mcp call error")
}

func TestConvertResponse_ToolSearchOutputUsesHostedCallID(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[
			{"type":"tool_search_call","id":"tsc_1","status":"completed","execution":"server","arguments":{"query":"docs"}},
			{"type":"tool_search_output","id":"tso_1","status":"completed","execution":"server","tools":[{"type":"function","name":"weather","parameters":{"type":"object"},"strict":true,"defer_loading":true}]}
		],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 2)
	assert.Equal(t, provider.ContentToolCall, res.Content[0].Type)
	assert.Equal(t, "tsc_1", res.Content[0].ToolCallID)
	assert.Equal(t, provider.ContentToolResult, res.Content[1].Type)
	assert.Equal(t, "tsc_1", res.Content[1].ToolCallID)
	assert.JSONEq(t, `{"tools":[{"type":"function","name":"weather","parameters":{"type":"object"},"strict":true,"defer_loading":true}]}`, string(res.Content[1].Result))
}

func TestConvertResponse_ShellUsesProviderToolSchema(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[
			{"type":"shell_call","id":"sh_1","call_id":"call_1","status":"completed","action":{"commands":["echo hi"],"timeout_ms":1000,"max_output_length":2048}},
			{"type":"shell_call_output","id":"sho_1","call_id":"call_1","status":"completed","output":[{"stdout":"hi\n","stderr":"","outcome":{"type":"exit","exit_code":0},"created_by":"sdk-only"}]}
		],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{isShellProviderExecuted: true})
	require.Len(t, res.Content, 2)
	assert.JSONEq(t, `{"action":{"commands":["echo hi"]}}`, string(res.Content[0].Input))
	assert.JSONEq(t, `{"output":[{"stdout":"hi\n","stderr":"","outcome":{"type":"exit","exitCode":0}}]}`, string(res.Content[1].Result))
}

func TestConvertResponse_ApplyPatchDeleteOmitsDiff(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"apply_patch_call","id":"ap_1","call_id":"call_1","status":"completed","operation":{"type":"delete_file","path":"old.txt"}}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 1)
	assert.JSONEq(t, `{"callId":"call_1","operation":{"type":"delete_file","path":"old.txt"}}`, string(res.Content[0].Input))
}

func TestConvertResponse_ShellProviderExecutedOnlyForContainerShell(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"shell_call","id":"sh_1","call_id":"call_1","status":"completed","action":{"commands":["echo hi"]}}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 1)
	assert.False(t, res.Content[0].ProviderExecuted)

	res = mustConvertResponse(t, resp, buildResult{isShellProviderExecuted: true})
	require.Len(t, res.Content, 1)
	assert.True(t, res.Content[0].ProviderExecuted)
}

func TestConvertResponse_FinishReasonWithFunctionCall(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"getWeather","arguments":"{\"city\":\"SF\"}"}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 1)
	assert.Equal(t, provider.ContentToolCall, res.Content[0].Type)
	assert.Equal(t, "call_1", res.Content[0].ToolCallID)
	assert.Equal(t, provider.FinishReasonToolCalls, res.FinishReason.Unified)
}

func TestConvertResponse_ImageGeneration(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"image_generation_call","id":"ig_1","status":"completed","result":"BASE64DATA"}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)
	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 2)
	assert.Equal(t, provider.ContentToolCall, res.Content[0].Type)
	assert.True(t, res.Content[0].ProviderExecuted)
	assert.Equal(t, "image_generation", res.Content[0].ToolName)
	assert.Equal(t, provider.ContentToolResult, res.Content[1].Type)
	assert.Contains(t, string(res.Content[1].Result), "BASE64DATA")
}

func TestConvertResponse_Compaction(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
		"output":[{"type":"compaction","id":"cmp_1","encrypted_content":"ENC"}],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)
	res := mustConvertResponse(t, resp, buildResult{})
	require.Len(t, res.Content, 1)
	assert.Equal(t, provider.ContentCustom, res.Content[0].Type)
	assert.Equal(t, "openai.compaction", res.Content[0].Kind)
	assert.JSONEq(t, `{"type":"compaction","itemId":"cmp_1","encryptedContent":"ENC"}`, string(res.Content[0].ProviderMetadata["openai"]))
}

func TestConvertResponse_ReasoningContextMetadata(t *testing.T) {
	resp := decodeResponse(t, `{
		"id":"resp_1","created_at":1,"model":"gpt-5.6","object":"response","status":"completed","reasoning":{"context":"all_turns"},"output":[],
		"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
	}`)

	res := mustConvertResponse(t, resp, buildResult{})
	require.Contains(t, res.ProviderMetadata, "openai")
	var meta map[string]any
	require.NoError(t, json.Unmarshal(res.ProviderMetadata["openai"], &meta))
	assert.Equal(t, "resp_1", meta["responseId"])
	assert.Equal(t, "all_turns", meta["reasoningContext"])
}

func TestConvertUsage_TokenSplit(t *testing.T) {
	t.Run("cache read", func(t *testing.T) {
		resp := decodeResponse(t, `{
			"id":"r","created_at":1,"model":"gpt-4o","object":"response","status":"completed","output":[],
			"usage":{"input_tokens":100,"output_tokens":50,"total_tokens":150,"input_tokens_details":{"cached_tokens":30},"output_tokens_details":{"reasoning_tokens":20}}
		}`)
		u := convertUsage(resp.Usage, nil)
		assert.Equal(t, 100, *u.InputTokens.Total)
		assert.Equal(t, 70, *u.InputTokens.NoCache)
		assert.Equal(t, 30, *u.InputTokens.CacheRead)
		assert.Nil(t, u.InputTokens.CacheWrite)
		assert.Equal(t, 50, *u.OutputTokens.Total)
		assert.Equal(t, 30, *u.OutputTokens.Text)
		assert.Equal(t, 20, *u.OutputTokens.Reasoning)
	})

	t.Run("cache write", func(t *testing.T) {
		resp := decodeResponse(t, `{
			"id":"r","created_at":1,"model":"gpt-5.6","object":"response","status":"completed","output":[],
			"usage":{"input_tokens":100,"output_tokens":50,"total_tokens":150,"input_tokens_details":{"cached_tokens":30,"cache_write_tokens":10},"output_tokens_details":{"reasoning_tokens":20}}
		}`)
		u := convertUsage(resp.Usage, nil)
		assert.Equal(t, 100, *u.InputTokens.Total)
		assert.Equal(t, 60, *u.InputTokens.NoCache)
		assert.Equal(t, 30, *u.InputTokens.CacheRead)
		require.NotNil(t, u.InputTokens.CacheWrite)
		assert.Equal(t, 10, *u.InputTokens.CacheWrite)
	})
}

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		reason          string
		hasFunctionCall bool
		want            provider.UnifiedFinishReason
	}{
		{"", false, provider.FinishReasonStop},
		{"", true, provider.FinishReasonToolCalls},
		{"max_output_tokens", false, provider.FinishReasonLength},
		{"content_filter", false, provider.FinishReasonContentFilter},
		{"weird", false, provider.FinishReasonOther},
		{"weird", true, provider.FinishReasonToolCalls},
	}
	for _, tc := range tests {
		got := mapFinishReason(tc.reason, tc.hasFunctionCall)
		assert.Equal(t, tc.want, got.Unified, "reason=%q hasFn=%v", tc.reason, tc.hasFunctionCall)
	}
}
