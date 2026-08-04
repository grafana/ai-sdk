package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unmarshalMessage(t *testing.T, raw string) *anthropic.BetaMessage {
	t.Helper()
	var msg anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &msg))
	return &msg
}

func TestConvertResponse_CarriesProviderAndModel(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [{"type": "text", "text": "hi"}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)

	for _, providerName := range []string{"anthropic", "anthropic.vertex"} {
		t.Run(providerName, func(t *testing.T) {
			result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, providerName, false)
			require.NoError(t, err)
			require.NotNil(t, result.Response)
			assert.Equal(t, providerName, result.Response.Provider)
			assert.Equal(t, "claude-sonnet-4-6", result.Response.ModelID)
		})
	}
}

func TestConvertResponse_FallbackProviderMetadata(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id":"msg_1",
		"type":"message",
		"role":"assistant",
		"model":"claude-opus-5",
		"content":[{"type":"text","text":"fallback answer"}],
		"stop_reason":"refusal",
		"stop_sequence":null,
		"stop_details":{"type":"refusal","category":"policy","explanation":"fallback unavailable","recommended_model":"claude-opus-4-8"},
		"container":null,
		"context_management":null,
		"usage":{"input_tokens":12,"output_tokens":5,"iterations":[{"type":"message","input_tokens":12,"output_tokens":0},{"type":"fallback_message","model":"claude-opus-4-8","input_tokens":12,"output_tokens":5}]}
	}`)

	result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
	require.NoError(t, err)
	require.NotNil(t, result.ProviderMetadata)
	assert.JSONEq(t, `{
		"usage":{"input_tokens":12,"output_tokens":5,"iterations":[{"type":"message","input_tokens":12,"output_tokens":0},{"type":"fallback_message","model":"claude-opus-4-8","input_tokens":12,"output_tokens":5}]},
		"stopSequence":null,
		"stopDetails":{"type":"refusal","category":"policy","explanation":"fallback unavailable","recommendedModel":"claude-opus-4-8"},
		"iterations":[{"type":"message","inputTokens":12,"outputTokens":0},{"type":"fallback_message","model":"claude-opus-4-8","inputTokens":12,"outputTokens":5}],
		"container":null,
		"contextManagement":null
	}`, string(result.ProviderMetadata["anthropic"]))
}

func TestBuildAnthropicProviderMetadata_PreservesEmptyStrings(t *testing.T) {
	metadata, err := buildAnthropicProviderMetadata(map[string]json.RawMessage{
		"stop_details": json.RawMessage(`{"type":"refusal","category":"","explanation":"","recommended_model":""}`),
	}, json.RawMessage(`{"input_tokens":1,"output_tokens":0,"iterations":[{"type":"fallback_message","model":"","input_tokens":1,"output_tokens":0}]}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"usage":{"input_tokens":1,"output_tokens":0,"iterations":[{"type":"fallback_message","model":"","input_tokens":1,"output_tokens":0}]},
		"stopSequence":null,
		"stopDetails":{"type":"refusal","category":"","explanation":"","recommendedModel":""},
		"iterations":[{"type":"fallback_message","model":"","inputTokens":1,"outputTokens":0}],
		"container":null,
		"contextManagement":null
	}`, string(metadata["anthropic"]))
}

func TestConvertResponse_ServerToolUse(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [
			{"type": "server_tool_use", "id": "stu_1", "name": "web_search", "input": {"query": "test"}}
		],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)

	mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
		ID:   "anthropic.web_search_20250305",
		Name: "search_docs",
	}})

	result, err := convertResponse(msg, mapping, false, nil, defaultGenerateID, "anthropic", false)
	require.NoError(t, err)

	require.Len(t, result.Content, 1)
	part := result.Content[0]
	assert.Equal(t, provider.ContentToolCall, part.Type)
	assert.Equal(t, "stu_1", part.ToolCallID)
	assert.Equal(t, "search_docs", part.ToolName)
	assert.True(t, part.ProviderExecuted, "expected ProviderExecuted=true")
}

func TestConvertResponse_WebSearchToolResult(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [
			{
				"type": "web_search_tool_result",
				"tool_use_id": "stu_1",
				"content": [
					{"type": "web_search_result", "title": "Test", "url": "https://example.com", "page_age": "2d", "encrypted_content": "abc"},
					{"type": "web_search_result", "title": "Other", "url": "https://other.com", "page_age": "", "encrypted_content": "def"}
				]
			}
		],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)

	mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
		ID:   "anthropic.web_search_20250305",
		Name: "search_docs",
	}})

	result, err := convertResponse(msg, mapping, false, nil, seqIDGenerator(), "anthropic", false)
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(result.Content), 3, "expected at least 3 content parts (1 tool-result + 2 sources)")

	toolResult := result.Content[0]
	assert.Equal(t, provider.ContentToolResult, toolResult.Type)
	assert.Equal(t, "stu_1", toolResult.ToolCallID)
	assert.Equal(t, "search_docs", toolResult.ToolName)
	require.NotNil(t, toolResult.Result, "part[0].Result should be non-nil")
	assert.Nil(t, toolResult.Input, "part[0].Input should be nil for tool-result (use Result field)")

	source1 := result.Content[1]
	assert.Equal(t, provider.ContentSource, source1.Type)
	assert.Equal(t, provider.SourceTypeURL, source1.SourceType)
	assert.Equal(t, "https://example.com", source1.URL)
	assert.Equal(t, "Test", source1.Text)
	assert.NotEmpty(t, source1.ID, "web search source should have non-empty ID")

	source2 := result.Content[2]
	assert.Equal(t, provider.ContentSource, source2.Type)
	assert.NotEmpty(t, source2.ID, "web search source should have non-empty ID")
	assert.NotEqual(t, source1.ID, source2.ID, "each web search source should have a unique ID")
}

func TestConvertResponse_WebFetchToolResult(t *testing.T) {
	t.Run("success_with_text_source", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-6",
			"content": [
				{
					"type": "web_fetch_tool_result",
					"tool_use_id": "stu_1",
					"content": {
						"type": "web_fetch_result",
						"url": "https://example.com",
						"retrieved_at": "2025-01-01T00:00:00Z",
						"content": {
							"type": "document",
							"title": "Example Page",
							"citations": {"enabled": true},
							"source": {
								"type": "text",
								"media_type": "text/plain",
								"data": "Hello world"
							}
						}
					}
				}
			],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.web_fetch_20250910",
			Name: "fetch_page",
		}})

		result, err := convertResponse(msg, mapping, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		part := result.Content[0]
		assert.Equal(t, provider.ContentToolResult, part.Type)
		assert.Equal(t, "stu_1", part.ToolCallID)
		assert.Equal(t, "fetch_page", part.ToolName)
		assert.False(t, part.IsError)

		var res map[string]any
		require.NoError(t, json.Unmarshal(part.Result, &res))
		assert.Equal(t, "web_fetch_result", res["type"])
		assert.Equal(t, "https://example.com", res["url"])
		assert.Equal(t, "2025-01-01T00:00:00Z", res["retrievedAt"])
		content := res["content"].(map[string]any)
		assert.Equal(t, "document", content["type"])
		assert.Equal(t, "Example Page", content["title"])
		source := content["source"].(map[string]any)
		assert.Equal(t, "text", source["type"])
		assert.Equal(t, "text/plain", source["mediaType"])
		assert.Equal(t, "Hello world", source["data"])
	})

	t.Run("error", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-6",
			"content": [
				{
					"type": "web_fetch_tool_result",
					"tool_use_id": "stu_2",
					"content": {
						"type": "web_fetch_tool_result_error",
						"error_code": "url_not_accessible"
					}
				}
			],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.web_fetch_20250910",
			Name: "fetch_page",
		}})

		result, err := convertResponse(msg, mapping, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		part := result.Content[0]
		assert.Equal(t, provider.ContentToolResult, part.Type)
		assert.Equal(t, "stu_2", part.ToolCallID)
		assert.Equal(t, "fetch_page", part.ToolName)
		assert.True(t, part.IsError)

		var errRes map[string]string
		require.NoError(t, json.Unmarshal(part.Result, &errRes))
		assert.Equal(t, "web_fetch_tool_result_error", errRes["type"])
		assert.Equal(t, "url_not_accessible", errRes["errorCode"])
	})
}

func TestConvertResponse_ToolSearchToolResult(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [
			{"type": "server_tool_use", "id": "stu_2", "name": "tool_search_tool_bm25", "input": {"query": "needle"}},
			{
				"type": "tool_search_tool_result",
				"tool_use_id": "stu_2",
				"content": {
					"type": "tool_search_tool_search_result",
					"tool_references": [{"type": "tool_reference", "name": "my_func", "description": "A function"}]
				}
			}
		],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)

	mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
		ID:   "anthropic.tool_search_bm25_20251119",
		Name: "search_tools",
	}})

	result, err := convertResponse(msg, mapping, false, nil, defaultGenerateID, "anthropic", false)
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(result.Content), 2)

	part := result.Content[1]
	assert.Equal(t, provider.ContentToolResult, part.Type)
	assert.Equal(t, "stu_2", part.ToolCallID)
	assert.Equal(t, "search_tools", part.ToolName)
	require.NotNil(t, part.Result, "expected Result to be non-nil")
	assert.Nil(t, part.Input, "Input should be nil for tool-result (use Result field)")
}

func TestConvertResponse_ServerToolUseUnknownNamePassesThrough(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [
			{"type": "server_tool_use", "id": "stu_1", "name": "future_tool", "input": {"query": "test"}}
		],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)

	result, err := convertResponse(msg, newToolNameMapping(nil), false, nil, defaultGenerateID, "anthropic", false)
	require.NoError(t, err)

	require.Len(t, result.Content, 1)
	assert.Equal(t, "future_tool", result.Content[0].ToolName)
	assert.True(t, result.Content[0].ProviderExecuted)
}

func TestConvertResponse_ToolSearchToolResult_FallbackMapping(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [
			{
				"type": "tool_search_tool_result",
				"tool_use_id": "stu_2",
				"content": {
					"type": "tool_search_tool_search_result",
					"tool_references": [{"type": "tool_reference", "name": "my_func", "description": "A function"}]
				}
			}
		],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)

	mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
		ID:   "anthropic.tool_search_regex_20251119",
		Name: "search_regex",
	}})

	result, err := convertResponse(msg, mapping, false, nil, defaultGenerateID, "anthropic", false)
	require.NoError(t, err)

	require.Len(t, result.Content, 1)
	assert.Equal(t, "search_regex", result.Content[0].ToolName)
}

func TestConvertResponse_MCPToolUse(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-6",
		"content": [
			{"type": "mcp_tool_use", "id": "tc_789", "name": "search_docs", "server_name": "docs-server", "input": {"query": "hello"}}
		],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)

	result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
	require.NoError(t, err)

	require.Len(t, result.Content, 1)
	part := result.Content[0]
	assert.Equal(t, provider.ContentToolCall, part.Type)
	assert.Equal(t, "tc_789", part.ToolCallID)
	assert.Equal(t, "search_docs", part.ToolName)
	assert.True(t, part.ProviderExecuted)
	assert.Equal(t, boolPtr(true), part.Dynamic)
	require.NotNil(t, part.ProviderMetadata)

	var meta map[string]string
	require.NoError(t, json.Unmarshal(part.ProviderMetadata["anthropic"], &meta))
	assert.Equal(t, "mcp-tool-use", meta["type"])
	assert.Equal(t, "docs-server", meta["serverName"])
}

func TestConvertResponse_MCPToolResult(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-6",
			"content": [
				{"type": "mcp_tool_use", "id": "tc_789", "name": "search_docs", "server_name": "docs-server", "input": {"query": "hello"}},
				{"type": "mcp_tool_result", "tool_use_id": "tc_789", "is_error": false, "content": [{"type": "text", "text": "Result data"}]}
			],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 2)
		part := result.Content[1]
		assert.Equal(t, provider.ContentToolResult, part.Type)
		assert.Equal(t, "tc_789", part.ToolCallID)
		assert.Equal(t, "search_docs", part.ToolName)
		assert.False(t, part.IsError)
		assert.Equal(t, boolPtr(true), part.Dynamic)
		require.NotNil(t, part.Result)
		assert.JSONEq(t, `[{"type":"text","text":"Result data"}]`, string(part.Result))
		require.NotNil(t, part.ProviderMetadata)
	})

	t.Run("error", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-6",
			"content": [
				{"type": "mcp_tool_use", "id": "tc_456", "name": "fail_tool", "server_name": "srv", "input": {}},
				{"type": "mcp_tool_result", "tool_use_id": "tc_456", "is_error": true, "content": "Tool execution failed"}
			],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 2)
		part := result.Content[1]
		assert.Equal(t, provider.ContentToolResult, part.Type)
		assert.True(t, part.IsError)
		assert.Equal(t, boolPtr(true), part.Dynamic)
		assert.JSONEq(t, `"Tool execution failed"`, string(part.Result))
	})

	t.Run("mixed_regular_and_mcp", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-6",
			"content": [
				{"type": "text", "text": "Let me search."},
				{"type": "tool_use", "id": "call_1", "name": "user_func", "input": {"x": 1}},
				{"type": "mcp_tool_use", "id": "tc_1", "name": "remote_tool", "server_name": "remote", "input": {"q": "test"}},
				{"type": "mcp_tool_result", "tool_use_id": "tc_1", "is_error": false, "content": "ok"}
			],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 4)

		assert.Equal(t, provider.ContentText, result.Content[0].Type)
		assert.Equal(t, provider.ContentToolCall, result.Content[1].Type)
		assert.False(t, result.Content[1].ProviderExecuted)
		assert.Nil(t, result.Content[1].Dynamic)

		assert.Equal(t, provider.ContentToolCall, result.Content[2].Type)
		assert.True(t, result.Content[2].ProviderExecuted)
		assert.Equal(t, boolPtr(true), result.Content[2].Dynamic)

		assert.Equal(t, provider.ContentToolResult, result.Content[3].Type)
		assert.Equal(t, boolPtr(true), result.Content[3].Dynamic)
	})
}

func TestConvertResponse_FinishReason(t *testing.T) {
	tests := []struct {
		reason  anthropic.BetaStopReason
		unified provider.UnifiedFinishReason
	}{
		{anthropic.BetaStopReasonPauseTurn, provider.FinishReasonStop},
		{anthropic.BetaStopReasonModelContextWindowExceeded, provider.FinishReasonLength},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			msg := unmarshalMessage(t, `{
				"id": "msg_1",
				"type": "message",
				"role": "assistant",
				"model": "claude-sonnet-4-6",
				"content": [{"type": "text", "text": "hi"}],
				"stop_reason": "`+string(tt.reason)+`",
				"usage": {"input_tokens": 10, "output_tokens": 5}
			}`)

			result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
			require.NoError(t, err)
			assert.Equal(t, provider.FinishReason{Unified: tt.unified, Raw: string(tt.reason)}, result.FinishReason)
		})
	}
}

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		reason anthropic.BetaStopReason
		want   provider.FinishReason
	}{
		{anthropic.BetaStopReasonPauseTurn, provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "pause_turn"}},
		{anthropic.BetaStopReasonEndTurn, provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"}},
		{anthropic.BetaStopReasonStopSequence, provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "stop_sequence"}},
		{anthropic.BetaStopReasonMaxTokens, provider.FinishReason{Unified: provider.FinishReasonLength, Raw: "max_tokens"}},
		{anthropic.BetaStopReasonModelContextWindowExceeded, provider.FinishReason{Unified: provider.FinishReasonLength, Raw: "model_context_window_exceeded"}},
		{anthropic.BetaStopReasonToolUse, provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_use"}},
		{"content_filter", provider.FinishReason{Unified: provider.FinishReasonContentFilter, Raw: "content_filter"}},
		{"refusal", provider.FinishReason{Unified: provider.FinishReasonContentFilter, Raw: "refusal"}},
		{"unknown_reason", provider.FinishReason{Unified: provider.FinishReasonOther, Raw: "unknown_reason"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			got := mapFinishReason(tt.reason)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertResponse_TextCitations(t *testing.T) {
	t.Run("web search citations produce source entries", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-6",
			"content": [
				{
					"type": "text",
					"text": "According to the source",
					"citations": [
						{"type": "web_search_result_location", "url": "https://example.com", "title": "Example", "cited_text": "some text", "encrypted_index": "enc1"}
					]
				}
			],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, newToolNameMapping(nil), false, nil, seqIDGenerator(), "anthropic", false)
		require.NoError(t, err)
		require.Len(t, result.Content, 2)

		assert.Equal(t, provider.ContentText, result.Content[0].Type)
		assert.Equal(t, "According to the source", result.Content[0].Text)
		assert.JSONEq(t, `{"citations":[{"type":"web_search_result_location","url":"https://example.com","title":"Example","cited_text":"some text","encrypted_index":"enc1"}]}`, string(result.Content[0].ProviderMetadata["anthropic"]))

		src := result.Content[1]
		assert.Equal(t, provider.ContentSource, src.Type)
		assert.NotEmpty(t, src.ID, "citation source should have non-empty ID")
		assert.Equal(t, provider.SourceTypeURL, src.SourceType)
		assert.Equal(t, "https://example.com", src.URL)
		assert.Equal(t, "Example", src.Text)
	})

	t.Run("document citations produce source entries", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-6",
			"content": [
				{
					"type": "text",
					"text": "The report says",
					"citations": [
						{"type": "page_location", "document_index": 0, "cited_text": "page text", "start_page_number": 1, "end_page_number": 2, "document_title": "", "file_id": "f1"}
					]
				}
			],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		docs := []citationDocument{
			{title: "Report", filename: "report.pdf", mediaType: "application/pdf"},
		}
		result, err := convertResponse(msg, newToolNameMapping(nil), false, docs, seqIDGenerator(), "anthropic", false)
		require.NoError(t, err)
		require.Len(t, result.Content, 2)

		src := result.Content[1]
		assert.Equal(t, provider.ContentSource, src.Type)
		assert.NotEmpty(t, src.ID, "citation source should have non-empty ID")
		assert.Equal(t, provider.SourceTypeDocument, src.SourceType)
		assert.Equal(t, "application/pdf", src.MediaType)
		assert.Equal(t, "Report", src.Text)
		assert.Equal(t, "report.pdf", src.Filename)
	})

	t.Run("no citations produces no extra entries", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-6",
			"content": [
				{
					"type": "text",
					"text": "Hello world",
					"citations": []
				}
			],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`)

		result, err := convertResponse(msg, newToolNameMapping(nil), false, nil, seqIDGenerator(), "anthropic", false)
		require.NoError(t, err)
		require.Len(t, result.Content, 1)
		assert.Equal(t, provider.ContentText, result.Content[0].Type)
	})
}

func TestConvertResponse_JsonResponseTool(t *testing.T) {
	t.Run("tool_use_remapped_to_text", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-3-haiku",
			"content": [
				{"type": "tool_use", "id": "call_json", "name": "json", "input": {"name": "Alice", "age": 30}}
			],
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 10, "output_tokens": 20}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, true, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		part := result.Content[0]
		assert.Equal(t, provider.ContentText, part.Type, "json tool_use should be remapped to text")
		assert.Contains(t, part.Text, `"name"`)
		assert.Contains(t, part.Text, `"Alice"`)
		assert.Empty(t, part.ToolCallID, "should not have a tool call ID")
		assert.Empty(t, part.ToolName, "should not have a tool name")
	})

	t.Run("finish_reason_remapped_to_stop", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-3-haiku",
			"content": [
				{"type": "tool_use", "id": "call_json", "name": "json", "input": {"result": "sunny"}}
			],
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 10, "output_tokens": 20}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, true, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		assert.Equal(t, provider.FinishReasonStop, result.FinishReason.Unified, "tool_use should be remapped to stop")
		assert.Equal(t, "tool_use", result.FinishReason.Raw, "raw reason should preserve original")
	})

	t.Run("regular_tools_unaffected", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-3-haiku",
			"content": [
				{"type": "tool_use", "id": "call_1", "name": "search", "input": {"q": "test"}},
				{"type": "tool_use", "id": "call_json", "name": "json", "input": {"answer": 42}}
			],
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 10, "output_tokens": 20}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, true, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 2)

		assert.Equal(t, provider.ContentToolCall, result.Content[0].Type, "regular tool should remain a tool-call")
		assert.Equal(t, "search", result.Content[0].ToolName)
		assert.Equal(t, "call_1", result.Content[0].ToolCallID)

		assert.Equal(t, provider.ContentText, result.Content[1].Type, "json tool should be remapped to text")
		assert.Contains(t, result.Content[1].Text, `"answer"`)
	})

	t.Run("text_blocks_suppressed", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-3-haiku",
			"content": [
				{"type": "text", "text": "Here is the JSON:"},
				{"type": "tool_use", "id": "call_json", "name": "json", "input": {"answer": 42}}
			],
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 10, "output_tokens": 20}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, true, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1, "text block should be suppressed, only json tool output")
		assert.Equal(t, provider.ContentText, result.Content[0].Type)
		assert.Contains(t, result.Content[0].Text, `"answer"`)
	})

	t.Run("finish_reason_not_remapped_when_user_tool_called", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-3-haiku",
			"content": [
				{"type": "tool_use", "id": "call_1", "name": "search", "input": {"q": "test"}}
			],
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 10, "output_tokens": 20}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, true, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		assert.Equal(t, provider.ContentToolCall, result.Content[0].Type)
		assert.Equal(t, provider.FinishReasonToolCalls, result.FinishReason.Unified,
			"when model calls a user tool (not json), finish reason should remain tool-calls")
	})

	t.Run("flag_disabled_no_remapping", func(t *testing.T) {
		msg := unmarshalMessage(t, `{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"model": "claude-3-haiku",
			"content": [
				{"type": "tool_use", "id": "call_json", "name": "json", "input": {"x": 1}}
			],
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 10, "output_tokens": 20}
		}`)

		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)

		require.Len(t, result.Content, 1)
		assert.Equal(t, provider.ContentToolCall, result.Content[0].Type, "without flag, json tool should be a normal tool-call")
		assert.Equal(t, "json", result.Content[0].ToolName)
		assert.Equal(t, provider.FinishReasonToolCalls, result.FinishReason.Unified, "finish reason should remain tool_calls without flag")
	})
}
