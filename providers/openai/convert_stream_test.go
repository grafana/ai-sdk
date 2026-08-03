package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unmarshalEvent decodes a single Responses stream event from JSON.
func unmarshalEvent(t *testing.T, raw string) responses.ResponseStreamEventUnion {
	t.Helper()
	var e responses.ResponseStreamEventUnion
	require.NoError(t, json.Unmarshal([]byte(raw), &e))
	return e
}

// collectParts drives the streamAdapter over the given events and returns the
// emitted parts.
func collectParts(t *testing.T, events ...string) []provider.StreamPart {
	t.Helper()
	return collectPartsWithBuildResult(t, buildResult{}, events...)
}

func collectPartsWithBuildResult(t *testing.T, br buildResult, events ...string) []provider.StreamPart {
	t.Helper()
	a := newStreamAdapter(nil, br, responses.ResponseNewParams{}, nil, seqIDGen(), "openai")
	ch := make(chan provider.StreamPart, 256)
	for _, raw := range events {
		a.handleEvent(unmarshalEvent(t, raw), ch)
	}
	close(ch)
	var parts []provider.StreamPart
	for p := range ch {
		parts = append(parts, p)
	}
	return parts
}

func partTypes(parts []provider.StreamPart) []provider.StreamPartType {
	var out []provider.StreamPartType
	for _, p := range parts {
		out = append(out, p.Type)
	}
	return out
}

func TestStream_TextLifecycle(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","created_at":1,"model":"gpt-4o","object":"response","status":"in_progress","output":[]}}`,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
		`{"type":"response.output_text.delta","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_1","delta":"Hel","logprobs":[]}`,
		`{"type":"response.output_text.delta","sequence_number":3,"output_index":0,"content_index":0,"item_id":"msg_1","delta":"lo","logprobs":[]}`,
		`{"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello","annotations":[]}]}}`,
		`{"type":"response.completed","sequence_number":5,"response":{"id":"resp_1","created_at":1,"model":"gpt-4o","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
	)

	assert.Equal(t, []provider.StreamPartType{
		provider.PartStreamStart,
		provider.PartResponseMeta,
		provider.PartTextStart,
		provider.PartTextDelta,
		provider.PartTextDelta,
		provider.PartTextEnd,
		provider.PartFinish,
	}, partTypes(parts))

	// Finish carries usage + stop reason.
	finish := parts[len(parts)-1]
	require.NotNil(t, finish.FinishReason)
	assert.Equal(t, provider.FinishReasonStop, finish.FinishReason.Unified)
	require.NotNil(t, finish.Usage)
}

func TestStream_FinishCarriesResponseMetadata(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.completed","sequence_number":1,"response":{"id":"resp_1","created_at":1,"model":"gpt-5.6","object":"response","status":"completed","reasoning":{"context":"all_turns"},"output":[],"usage":{"input_tokens":100,"output_tokens":50,"total_tokens":150,"input_tokens_details":{"cached_tokens":30,"cache_write_tokens":10},"output_tokens_details":{"reasoning_tokens":20}}}}`,
	)

	var finish provider.StreamPart
	for _, part := range parts {
		if part.Type == provider.PartFinish {
			finish = part
		}
	}
	assert.Equal(t, provider.PartFinish, finish.Type)
	require.NotNil(t, finish.Usage)
	assert.Equal(t, 60, *finish.Usage.InputTokens.NoCache)
	require.NotNil(t, finish.Usage.InputTokens.CacheWrite)
	assert.Equal(t, 10, *finish.Usage.InputTokens.CacheWrite)
	require.Contains(t, finish.ProviderMetadata, "openai")
	var meta map[string]any
	require.NoError(t, json.Unmarshal(finish.ProviderMetadata["openai"], &meta))
	assert.Equal(t, "resp_1", meta["responseId"])
	assert.Equal(t, "all_turns", meta["reasoningContext"])
}

func TestStream_TextEndCarriesAnnotations(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
		`{"type":"response.output_text.delta","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_1","delta":"hi","logprobs":[]}`,
		`{"type":"response.output_text.annotation.added","sequence_number":3,"output_index":0,"content_index":0,"item_id":"msg_1","annotation_index":0,"annotation":{"type":"url_citation","url":"https://example.com","title":"Example","start_index":0,"end_index":2}}`,
		`{"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hi","annotations":[]}]}}`,
	)

	var textEnd *provider.StreamPart
	for i := range parts {
		if parts[i].Type == provider.PartTextEnd {
			textEnd = &parts[i]
		}
	}
	require.NotNil(t, textEnd)
	raw, ok := textEnd.ProviderMetadata["openai"]
	require.True(t, ok, "openai metadata present")
	var meta map[string]any
	require.NoError(t, json.Unmarshal(raw, &meta))
	assert.Equal(t, "msg_1", meta["itemId"])
	annotations, ok := meta["annotations"].([]any)
	require.True(t, ok, "annotations present in text-end metadata")
	require.Len(t, annotations, 1)
	ann := annotations[0].(map[string]any)
	assert.Equal(t, "url_citation", ann["type"])
	assert.Equal(t, "https://example.com", ann["url"])
}

func TestStream_AnnotationEmitsSource(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
		`{"type":"response.output_text.annotation.added","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_1","annotation_index":0,"annotation":{"type":"url_citation","url":"https://example.com","title":"Example","start_index":0,"end_index":2}}`,
	)

	var source *provider.StreamPart
	for i := range parts {
		if parts[i].Type == provider.PartSource {
			source = &parts[i]
		}
	}
	require.NotNil(t, source)
	require.NotNil(t, source.Source)
	assert.Equal(t, provider.SourceTypeURL, source.Source.SourceType)
	assert.Equal(t, "https://example.com", source.Source.URL)
	assert.Equal(t, "Example", source.Source.Title)
}

func TestStream_ErrorThenResponseFailedEmitsSingleErrorAndFinish(t *testing.T) {
	parts := collectParts(t,
		`{"type":"error","sequence_number":1,"message":"stream failed","code":"rate_limit_error","param":null}`,
		`{"type":"response.failed","sequence_number":2,"response":{"id":"resp_1","model":"gpt-4o","status":"failed","error":{"message":"stream failed","code":"rate_limit_error"},"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}}`,
	)

	var errorsSeen, finishesSeen int
	for _, part := range parts {
		switch part.Type {
		case provider.PartError:
			errorsSeen++
		case provider.PartFinish:
			finishesSeen++
			require.NotNil(t, part.FinishReason)
			assert.Equal(t, provider.FinishReasonError, part.FinishReason.Unified)
			require.NotNil(t, part.Usage)
			require.NotNil(t, part.Usage.InputTokens.Total)
			assert.Equal(t, 3, *part.Usage.InputTokens.Total)
			assert.JSONEq(t, `{"responseId":"resp_1"}`, string(part.ProviderMetadata["openai"]))
		}
	}
	assert.Equal(t, 1, errorsSeen)
	assert.Equal(t, 1, finishesSeen)
}

func TestStream_TextCarriesPhaseMetadata(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","phase":"commentary","status":"in_progress","content":[]}}`,
		`{"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hi","annotations":[]}]}}`,
	)

	var textStart, textEnd *provider.StreamPart
	for i := range parts {
		switch parts[i].Type {
		case provider.PartTextStart:
			textStart = &parts[i]
		case provider.PartTextEnd:
			textEnd = &parts[i]
		}
	}
	require.NotNil(t, textStart)
	require.NotNil(t, textEnd)

	var startMeta map[string]any
	require.NoError(t, json.Unmarshal(textStart.ProviderMetadata["openai"], &startMeta))
	assert.Equal(t, "commentary", startMeta["phase"])

	var endMeta map[string]any
	require.NoError(t, json.Unmarshal(textEnd.ProviderMetadata["openai"], &endMeta))
	assert.Equal(t, "commentary", endMeta["phase"])
}

func TestStream_WebSearchOutputIncludesQueries(t *testing.T) {
	a := newStreamAdapter(nil, buildResult{}, responses.ResponseNewParams{}, nil, seqIDGen(), "openai")
	ch := make(chan provider.StreamPart, 64)
	a.handleEvent(unmarshalEvent(t,
		`{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"go release year","queries":["go release year"]}}}`,
	), ch)
	close(ch)

	var result *provider.StreamPart
	for p := range ch {
		if p.Type == provider.PartToolResult {
			pp := p
			result = &pp
		}
	}
	require.NotNil(t, result)
	require.NotEmpty(t, result.Result)
	var out map[string]any
	require.NoError(t, json.Unmarshal(result.Result, &out))
	action := out["action"].(map[string]any)
	assert.Equal(t, "search", action["type"])
	assert.Equal(t, "go release year", action["query"])
	assert.Equal(t, []any{"go release year"}, action["queries"])
}

func TestStream_WebSearchPreviewUsesCustomToolName(t *testing.T) {
	parts := collectPartsWithBuildResult(t, buildResult{webSearchToolName: "search"},
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"in_progress","action":{"type":"search","query":"go"}}}`,
		`{"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"go"}}}`,
	)

	var names []string
	for _, part := range parts {
		if part.Type == provider.PartToolInputStart || part.Type == provider.PartToolCall || part.Type == provider.PartToolResult {
			names = append(names, part.ToolName)
		}
	}
	assert.Equal(t, []string{"search", "search", "search"}, names)
}

func TestStream_FunctionCall(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"getWeather","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","sequence_number":2,"output_index":0,"item_id":"fc_1","delta":"{\"city\":"}`,
		`{"type":"response.function_call_arguments.delta","sequence_number":3,"output_index":0,"item_id":"fc_1","delta":"\"SF\"}"}`,
		`{"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"getWeather","arguments":"{\"city\":\"SF\"}","status":"completed"}}`,
	)

	assert.Equal(t, []provider.StreamPartType{
		provider.PartStreamStart,
		provider.PartToolInputStart,
		provider.PartToolInputDelta,
		provider.PartToolInputDelta,
		provider.PartToolInputEnd,
		provider.PartToolCall,
	}, partTypes(parts))

	call := parts[len(parts)-1]
	assert.Equal(t, "call_1", call.ToolCallID)
	assert.Equal(t, "getWeather", call.ToolName)
	assert.Equal(t, `{"city":"SF"}`, call.Input)
}

func TestStream_ComputerCall(t *testing.T) {
	mapping := newToolNameMapping([]provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDComputer, Name: "browser"}})
	parts := collectPartsWithBuildResult(t, buildResult{hasComputerTool: true, toolNameMapping: mapping},
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"computer_call","id":"item_1","call_id":"call_1","status":"in_progress","actions":[],"pending_safety_checks":[]}}`,
		`{"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"computer_call","id":"item_1","call_id":"call_1","status":"completed","actions":[{"type":"click","button":"left","x":1,"y":2},{"type":"scroll","x":3,"y":4,"scroll_x":5,"scroll_y":6}],"pending_safety_checks":[{"id":"safe_1"}]}}`,
		`{"type":"response.completed","sequence_number":3,"response":{"id":"resp_1","created_at":1,"model":"computer-preview","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
	)

	assert.Equal(t, []provider.StreamPartType{
		provider.PartStreamStart,
		provider.PartToolInputStart,
		provider.PartToolInputDelta,
		provider.PartToolInputEnd,
		provider.PartToolCall,
		provider.PartFinish,
	}, partTypes(parts))
	call := parts[4]
	assert.Equal(t, "call_1", call.ToolCallID)
	assert.Equal(t, "browser", call.ToolName)
	assert.False(t, call.ProviderExecuted)
	assert.JSONEq(t, `{"actions":[{"type":"click","button":"left","x":1,"y":2},{"type":"scroll","x":3,"y":4,"scrollX":5,"scrollY":6}],"pendingSafetyChecks":[{"id":"safe_1"}],"status":"completed"}`, call.Input)
	assert.Equal(t, call.Input, parts[2].Delta)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal(call.ProviderMetadata["openai"], &metadata))
	assert.Equal(t, "item_1", metadata["itemId"])
	assert.Equal(t, provider.FinishReasonToolCalls, parts[5].FinishReason.Unified)
}

func TestStream_ComputerCallPrefersExplicitActions(t *testing.T) {
	mapping := newToolNameMapping([]provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDComputer, Name: "browser"}})
	parts := collectPartsWithBuildResult(t, buildResult{hasComputerTool: true, toolNameMapping: mapping},
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"computer_call","id":"item_1","call_id":"call_1","status":"in_progress","actions":[],"action":{"type":"screenshot"},"pending_safety_checks":[]}}`,
		`{"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"computer_call","id":"item_1","call_id":"call_1","status":"completed","actions":[],"action":{"type":"screenshot"},"pending_safety_checks":[]}}`,
	)

	require.Len(t, parts, 5)
	assert.JSONEq(t, `{"actions":[],"pendingSafetyChecks":[],"status":"completed"}`, string(parts[4].Input))
}

func TestStream_ComputerCallFallsBackFromNullActions(t *testing.T) {
	mapping := newToolNameMapping([]provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDComputer, Name: "browser"}})
	parts := collectPartsWithBuildResult(t, buildResult{hasComputerTool: true, toolNameMapping: mapping},
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"computer_call","id":"item_1","call_id":"call_1","status":"in_progress","actions":null,"action":{"type":"screenshot"},"pending_safety_checks":[]}}`,
		`{"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"computer_call","id":"item_1","call_id":"call_1","status":"completed","actions":null,"action":{"type":"screenshot"},"pending_safety_checks":[]}}`,
	)

	require.Len(t, parts, 5)
	assert.JSONEq(t, `{"actions":[{"type":"screenshot"}],"pendingSafetyChecks":[],"status":"completed"}`, string(parts[4].Input))
}

func TestStream_LegacyComputerCall(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"computer_call","id":"item_1","call_id":null,"status":"in_progress","action":{"type":"screenshot"},"pending_safety_checks":[]}}`,
		`{"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"computer_call","id":"item_1","call_id":null,"status":"completed","action":{"type":"screenshot"},"pending_safety_checks":[]}}`,
	)

	assert.Equal(t, []provider.StreamPartType{
		provider.PartStreamStart,
		provider.PartToolInputStart,
		provider.PartToolInputEnd,
		provider.PartToolCall,
		provider.PartToolResult,
	}, partTypes(parts))
	assert.Equal(t, "computer", parts[1].ToolName)
	assert.False(t, parts[1].ProviderExecuted)
	assert.True(t, parts[3].ProviderExecuted)
	assert.Equal(t, "computer_use", parts[3].ToolName)
}

func TestStream_FunctionCallNamespaceMetadata(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"getWeather","arguments":""}}`,
		`{"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"getWeather","namespace":"weather_ns","arguments":"{}","status":"completed"}}`,
	)

	var toolCall *provider.StreamPart
	for i := range parts {
		if parts[i].Type == provider.PartToolCall {
			toolCall = &parts[i]
		}
	}
	require.NotNil(t, toolCall)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(toolCall.ProviderMetadata["openai"], &meta))
	assert.Equal(t, "fc_1", meta["itemId"])
	assert.Equal(t, "weather_ns", meta["namespace"])
}

func TestStream_WebSearchEagerCall(t *testing.T) {
	a := newStreamAdapter(nil, buildResult{}, responses.ResponseNewParams{}, nil, seqIDGen(), "openai")
	ch := make(chan provider.StreamPart, 64)
	a.handleEvent(unmarshalEvent(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"in_progress","action":{"type":"search","query":"go"}}}`,
	), ch)
	close(ch)
	var parts []provider.StreamPart
	for p := range ch {
		parts = append(parts, p)
	}

	assert.Equal(t, []provider.StreamPartType{
		provider.PartStreamStart,
		provider.PartToolInputStart,
		provider.PartToolInputEnd,
		provider.PartToolCall,
	}, partTypes(parts))
	last := parts[len(parts)-1]
	assert.True(t, last.ProviderExecuted)
	assert.Equal(t, "web_search", last.ToolName)
}

func TestStream_ToolSearchOutputUsesHostedCallID(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"tool_search_call","id":"tsc_1","status":"in_progress","execution":"server","arguments":{}}}`,
		`{"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"tool_search_call","id":"tsc_1","status":"completed","execution":"server","arguments":{"query":"docs"}}}`,
		`{"type":"response.output_item.done","sequence_number":3,"output_index":1,"item":{"type":"tool_search_output","id":"tso_1","status":"completed","execution":"server","tools":[]}}`,
	)

	var toolCall, toolResult *provider.StreamPart
	for i := range parts {
		switch parts[i].Type {
		case provider.PartToolCall:
			toolCall = &parts[i]
		case provider.PartToolResult:
			toolResult = &parts[i]
		}
	}
	require.NotNil(t, toolCall)
	require.NotNil(t, toolResult)
	assert.Equal(t, "tsc_1", toolCall.ToolCallID)
	assert.Equal(t, "tsc_1", toolResult.ToolCallID)
	var input map[string]any
	require.NoError(t, json.Unmarshal([]byte(toolCall.Input), &input))
	assert.Contains(t, input, "call_id")
	assert.Nil(t, input["call_id"])
}

func TestStream_MCPCallResultFieldPresence(t *testing.T) {
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
			parts := collectPartsWithBuildResult(t, br,
				`{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"mcp_call","id":"mcp_1","name":"do_thing","server_label":"srv","arguments":"{}"`+approvalRequest+tc.fields+`}}`,
			)

			var toolCall, toolResult *provider.StreamPart
			for i := range parts {
				switch parts[i].Type {
				case provider.PartToolCall:
					toolCall = &parts[i]
				case provider.PartToolResult:
					toolResult = &parts[i]
				}
			}
			require.NotNil(t, toolCall)
			assert.Nil(t, toolCall.ProviderMetadata)
			require.NotNil(t, toolResult)
			assert.Equal(t, toolCallID, toolResult.ToolCallID)
			assert.JSONEq(t, `{"itemId":"mcp_1"}`, string(toolResult.ProviderMetadata["openai"]))
			var result map[string]any
			require.NoError(t, json.Unmarshal(toolResult.Result, &result))
			assert.Equal(t, tc.want, result)
		})
	}
}

func TestStream_MCPCallResultConversionError(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"mcp_call","id":"mcp_1","name":"do_thing","server_label":"srv","arguments":"{}","error":[]}}`,
	)

	require.Len(t, parts, 2)
	assert.Equal(t, provider.PartStreamStart, parts[0].Type)
	assert.Equal(t, provider.PartError, parts[1].Type)
	require.NotNil(t, parts[1].APICallError)
	assert.Contains(t, parts[1].APICallError.Message, "openai: decoding mcp call error")
}

func TestStream_MCPCallUsesApprovalToolCallID(t *testing.T) {
	parts := collectPartsWithBuildResult(t, buildResult{
		approvalRequestToolCallIDs: map[string]string{"appr_1": "dummy_call_1"},
	},
		`{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"mcp_call","id":"mcp_1","approval_request_id":"appr_1","name":"do_thing","server_label":"srv","arguments":"{}","output":"ok"}}`,
	)

	var toolCall, toolResult *provider.StreamPart
	for i := range parts {
		switch parts[i].Type {
		case provider.PartToolCall:
			toolCall = &parts[i]
		case provider.PartToolResult:
			toolResult = &parts[i]
		}
	}
	require.NotNil(t, toolCall)
	require.NotNil(t, toolResult)
	assert.Equal(t, "dummy_call_1", toolCall.ToolCallID)
	assert.Equal(t, "dummy_call_1", toolResult.ToolCallID)
	assert.JSONEq(t, `{"type":"call","serverLabel":"srv","name":"do_thing","arguments":"{}","output":"ok"}`, string(toolResult.Result))
	assert.JSONEq(t, `{"itemId":"mcp_1"}`, string(toolResult.ProviderMetadata["openai"]))
	assert.NotContains(t, string(toolResult.Result), `"error"`)
}

func TestStream_MCPCallPreservesNullableFieldPresence(t *testing.T) {
	cases := []struct {
		name     string
		event    string
		expected string
	}{
		{
			name:     "absent",
			event:    `{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"mcp_call","id":"mcp_1","name":"do_thing","server_label":"srv","arguments":"{}"}}`,
			expected: `{"type":"call","serverLabel":"srv","name":"do_thing","arguments":"{}"}`,
		},
		{
			name:     "null",
			event:    `{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"mcp_call","id":"mcp_1","name":"do_thing","server_label":"srv","arguments":"{}","output":null,"error":null}}`,
			expected: `{"type":"call","serverLabel":"srv","name":"do_thing","arguments":"{}"}`,
		},
		{
			name:     "empty output",
			event:    `{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"mcp_call","id":"mcp_1","name":"do_thing","server_label":"srv","arguments":"{}","output":""}}`,
			expected: `{"type":"call","serverLabel":"srv","name":"do_thing","arguments":"{}","output":""}`,
		},
		{
			name:     "empty error",
			event:    `{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"mcp_call","id":"mcp_1","name":"do_thing","server_label":"srv","arguments":"{}","error":""}}`,
			expected: `{"type":"call","serverLabel":"srv","name":"do_thing","arguments":"{}","error":""}`,
		},
		{
			name:     "non-empty output and error",
			event:    `{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"mcp_call","id":"mcp_1","name":"do_thing","server_label":"srv","arguments":"{}","output":"ok","error":"failed"}}`,
			expected: `{"type":"call","serverLabel":"srv","name":"do_thing","arguments":"{}","output":"ok","error":"failed"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts := collectParts(t, tc.event)
			var toolResult *provider.StreamPart
			for i := range parts {
				if parts[i].Type == provider.PartToolResult {
					toolResult = &parts[i]
				}
			}
			require.NotNil(t, toolResult)
			assert.JSONEq(t, tc.expected, string(toolResult.Result))
		})
	}
}

func TestStream_MCPCallUsesSameStreamApprovalToolCallID(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":{"type":"mcp_approval_request","id":"appr_1","approval_request_id":"appr_1","name":"do_thing","server_label":"srv","arguments":"{}"}}`,
		`{"type":"response.output_item.done","sequence_number":2,"output_index":1,"item":{"type":"mcp_call","id":"mcp_1","approval_request_id":"appr_1","name":"do_thing","server_label":"srv","arguments":"{}","output":"ok"}}`,
	)

	var approvalToolCallID string
	var resultToolCallID string
	for _, part := range parts {
		switch part.Type {
		case provider.PartToolApprovalRequest:
			approvalToolCallID = part.ToolCallID
		case provider.PartToolResult:
			resultToolCallID = part.ToolCallID
		}
	}
	require.NotEmpty(t, approvalToolCallID)
	assert.Equal(t, approvalToolCallID, resultToolCallID)
}

func TestStream_CodeInterpreterCodeDoneEmitsToolCallBeforeResult(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"code_interpreter_call","id":"ci_1","status":"in_progress","container_id":"ctr_1","code":"","outputs":[]}}`,
		`{"type":"response.code_interpreter_call_code.delta","sequence_number":2,"output_index":0,"item_id":"ci_1","delta":" <\n"}`,
		`{"type":"response.code_interpreter_call_code.done","sequence_number":3,"output_index":0,"item_id":"ci_1","code":" <\n"}`,
		`{"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"type":"code_interpreter_call","id":"ci_1","status":"completed","container_id":"ctr_1","code":" <\n","outputs":[]}}`,
	)

	assert.Equal(t, []provider.StreamPartType{
		provider.PartStreamStart,
		provider.PartToolInputStart,
		provider.PartToolInputDelta,
		provider.PartToolInputDelta,
		provider.PartToolInputDelta,
		provider.PartToolInputEnd,
		provider.PartToolCall,
		provider.PartToolResult,
	}, partTypes(parts))
	assert.Equal(t, ` <\n`, parts[3].Delta)
	assert.JSONEq(t, `{"code":" <\n","containerId":"ctr_1"}`, parts[6].Input)
}

func TestStream_ImageGenerationPartialImage(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.image_generation_call.partial_image","sequence_number":1,"output_index":0,"item_id":"ig_1","partial_image_b64":"BASE64","partial_image_index":0}`,
	)

	require.Len(t, parts, 2)
	part := parts[1]
	assert.Equal(t, provider.PartToolResult, part.Type)
	assert.Equal(t, "ig_1", part.ToolCallID)
	require.NotNil(t, part.Preliminary)
	assert.True(t, *part.Preliminary)
}

func TestStream_ApplyPatchDiffEvents(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"apply_patch_call","id":"ap_1","call_id":"call_1","status":"in_progress","operation":{"type":"update_file","path":"main.go","diff":""}}}`,
		`{"type":"response.apply_patch_call_operation_diff.delta","sequence_number":2,"output_index":0,"delta":"@@ -1 +1"}`,
		`{"type":"response.apply_patch_call_operation_diff.done","sequence_number":3,"output_index":0,"diff":"@@ -1 +1"}`,
		`{"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"type":"apply_patch_call","id":"ap_1","call_id":"call_1","status":"completed","operation":{"type":"update_file","path":"main.go","diff":"@@ -1 +1"}}}`,
	)

	assert.Contains(t, partTypes(parts), provider.PartToolCall)
}

func TestStream_ProviderExecutedToolNameMapping(t *testing.T) {
	mapping := newToolNameMapping([]provider.Tool{
		{Type: provider.ToolTypeProvider, ID: toolIDFileSearch, Name: "docs"},
		{Type: provider.ToolTypeProvider, ID: toolIDCodeInterpreter, Name: "python"},
		{Type: provider.ToolTypeProvider, ID: toolIDImageGeneration, Name: "draw"},
	})
	parts := collectPartsWithBuildResult(t, buildResult{toolNameMapping: mapping},
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"file_search_call","id":"fs_1","status":"in_progress","queries":[],"results":[]}}`,
		`{"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"type":"file_search_call","id":"fs_1","status":"completed","queries":["q"],"results":[]}}`,
		`{"type":"response.output_item.added","sequence_number":3,"output_index":1,"item":{"type":"code_interpreter_call","id":"ci_1","status":"in_progress","container_id":"ctr_1","code":"","outputs":[]}}`,
		`{"type":"response.output_item.done","sequence_number":4,"output_index":1,"item":{"type":"code_interpreter_call","id":"ci_1","status":"completed","container_id":"ctr_1","code":"print(1)","outputs":[]}}`,
		`{"type":"response.output_item.added","sequence_number":5,"output_index":2,"item":{"type":"image_generation_call","id":"ig_1","status":"in_progress","result":null}}`,
		`{"type":"response.output_item.done","sequence_number":6,"output_index":2,"item":{"type":"image_generation_call","id":"ig_1","status":"completed","result":"BASE64DATA"}}`,
	)

	var toolNames []string
	for _, part := range parts {
		if part.Type == provider.PartToolCall || part.Type == provider.PartToolInputStart || part.Type == provider.PartToolResult {
			toolNames = append(toolNames, part.ToolName)
		}
	}
	assert.Contains(t, toolNames, "docs")
	assert.Contains(t, toolNames, "python")
	assert.Contains(t, toolNames, "draw")
	assert.NotContains(t, toolNames, "file_search")
	assert.NotContains(t, toolNames, "code_interpreter")
	assert.NotContains(t, toolNames, "image_generation")
}

func TestStream_UnknownEventIgnored(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.audio.delta","sequence_number":1,"delta":"x"}`,
	)
	// Only the stream-start is emitted; the unknown event produces nothing.
	assert.Equal(t, []provider.StreamPartType{provider.PartStreamStart}, partTypes(parts))
}

func TestStream_ReasoningSummary(t *testing.T) {
	parts := collectParts(t,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":null}}`,
		`{"type":"response.reasoning_summary_text.delta","sequence_number":2,"output_index":0,"item_id":"rs_1","summary_index":0,"delta":"thinking"}`,
		`{"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"thinking"}],"encrypted_content":null}}`,
	)
	assert.Equal(t, []provider.StreamPartType{
		provider.PartStreamStart,
		provider.PartReasoningStart,
		provider.PartReasoningDelta,
		provider.PartReasoningEnd,
	}, partTypes(parts))
	assert.Equal(t, "rs_1:0", parts[1].ID)
	assert.Equal(t, "rs_1:0", parts[2].ID)

	raw, ok := parts[3].ProviderMetadata["openai"]
	require.True(t, ok)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(raw, &meta))
	assert.Contains(t, meta, "reasoningEncryptedContent")
	assert.Nil(t, meta["reasoningEncryptedContent"])
}

func TestStream_ReasoningSummaryPartsFollowStoreSemantics(t *testing.T) {
	parts := collectPartsWithBuildResult(t, buildResult{store: false},
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"enc"}}`,
		`{"type":"response.reasoning_summary_text.delta","sequence_number":2,"output_index":0,"item_id":"rs_1","summary_index":0,"delta":"one"}`,
		`{"type":"response.reasoning_summary_part.done","sequence_number":3,"output_index":0,"item_id":"rs_1","summary_index":0,"part":{"type":"summary_text","text":"one"}}`,
		`{"type":"response.reasoning_summary_part.added","sequence_number":4,"output_index":0,"item_id":"rs_1","summary_index":1,"part":{"type":"summary_text","text":""}}`,
		`{"type":"response.reasoning_summary_text.delta","sequence_number":5,"output_index":0,"item_id":"rs_1","summary_index":1,"delta":"two"}`,
		`{"type":"response.output_item.done","sequence_number":6,"output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"one"},{"type":"summary_text","text":"two"}],"encrypted_content":"enc"}}`,
	)

	var reasoningIDs []string
	for _, part := range parts {
		if part.Type == provider.PartReasoningStart || part.Type == provider.PartReasoningEnd {
			reasoningIDs = append(reasoningIDs, string(part.Type)+":"+part.ID)
		}
	}
	assert.Equal(t, []string{
		"reasoning-start:rs_1:0",
		"reasoning-end:rs_1:0",
		"reasoning-start:rs_1:1",
		"reasoning-end:rs_1:1",
	}, reasoningIDs)

	raw, ok := parts[3].ProviderMetadata["openai"]
	require.True(t, ok)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(raw, &meta))
	assert.Equal(t, "enc", meta["reasoningEncryptedContent"])
}
