package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unmarshalEvent(t *testing.T, raw string) anthropic.BetaRawMessageStreamEventUnion {
	t.Helper()
	var event anthropic.BetaRawMessageStreamEventUnion
	require.NoError(t, json.Unmarshal([]byte(raw), &event))
	return event
}

func collectParts(events []anthropic.BetaRawMessageStreamEventUnion) []provider.StreamPart {
	return collectPartsWithOpts(events, toolNameMapping{}, nil, nil)
}

func collectPartsWithMapping(events []anthropic.BetaRawMessageStreamEventUnion, mapping toolNameMapping) []provider.StreamPart {
	return collectPartsWithOpts(events, mapping, nil, nil)
}

func collectPartsWithOpts(events []anthropic.BetaRawMessageStreamEventUnion, mapping toolNameMapping, citDocs []citationDocument, genID func() string) []provider.StreamPart {
	if genID == nil {
		genID = defaultGenerateID
	}
	adapter := &streamAdapter{
		blocks:            make(map[int64]*blockState),
		mapping:           mapping,
		serverToolCalls:   make(map[string]string),
		mcpToolCalls:      make(map[string]mcpToolCallInfo),
		citationDocuments: citDocs,
		generateID:        genID,
	}
	ch := make(chan provider.StreamPart, 100)
	for _, e := range events {
		_ = adapter.handleEvent(e, ch)
	}
	close(ch)

	var parts []provider.StreamPart
	for p := range ch {
		parts = append(parts, p)
	}
	return parts
}

func TestStreamAdapter_FallbackProviderMetadata(t *testing.T) {
	events := []anthropic.BetaRawMessageStreamEventUnion{
		unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5","content":[],"stop_reason":null,"stop_sequence":null,"stop_details":null,"container":{"id":"container-1","expires_at":"2026-08-03T15:00:00Z"},"context_management":null,"usage":{"input_tokens":12,"output_tokens":0}}}`),
		unmarshalEvent(t, `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null,"stop_details":null},"usage":{"input_tokens":12,"output_tokens":5,"iterations":[{"type":"message","input_tokens":12,"output_tokens":0},{"type":"fallback_message","model":"claude-opus-4-8","input_tokens":12,"output_tokens":5}]}}`),
	}

	parts := collectParts(events)
	require.Len(t, parts, 2)
	finish := parts[1]
	assert.Equal(t, provider.PartFinish, finish.Type)
	require.NotNil(t, finish.ProviderMetadata)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal(finish.ProviderMetadata["anthropic"], &metadata))
	assert.Equal(t, []any{
		map[string]any{"type": "message", "inputTokens": float64(12), "outputTokens": float64(0)},
		map[string]any{"type": "fallback_message", "model": "claude-opus-4-8", "inputTokens": float64(12), "outputTokens": float64(5)},
	}, metadata["iterations"])
	assert.Nil(t, metadata["container"])
}

func TestStreamAdapter_ContentLifecycle(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 4)
		assert.Equal(t, provider.PartTextStart, parts[0].Type)
		assert.Equal(t, "0", parts[0].ID)
		assert.Equal(t, provider.PartTextDelta, parts[1].Type)
		assert.Equal(t, "0", parts[1].ID)
		assert.Equal(t, "Hello", parts[1].Delta)
		assert.Equal(t, provider.PartTextDelta, parts[2].Type)
		assert.Equal(t, "0", parts[2].ID)
		assert.Equal(t, " world", parts[2].Delta)
		assert.Equal(t, provider.PartTextEnd, parts[3].Type)
		assert.Equal(t, "0", parts[3].ID)
	})

	t.Run("reasoning", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 3)
		assert.Equal(t, provider.PartReasoningStart, parts[0].Type)
		assert.Equal(t, "0", parts[0].ID)
		assert.Equal(t, provider.PartReasoningDelta, parts[1].Type)
		assert.Equal(t, "0", parts[1].ID)
		assert.Equal(t, "Let me think", parts[1].Delta)
		assert.Equal(t, provider.PartReasoningEnd, parts[2].Type)
		assert.Equal(t, "0", parts[2].ID)
	})

	t.Run("tool_use", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"search"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"test\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 5)

		assert.Equal(t, provider.PartToolInputStart, parts[0].Type)
		assert.Equal(t, "call_1", parts[0].ID)
		assert.Equal(t, "search", parts[0].ToolName)

		assert.Equal(t, provider.PartToolInputDelta, parts[1].Type)
		assert.Equal(t, "call_1", parts[1].ID)

		assert.Equal(t, provider.PartToolInputDelta, parts[2].Type)

		assert.Equal(t, provider.PartToolInputEnd, parts[3].Type)
		assert.Equal(t, "call_1", parts[3].ID)

		assert.Equal(t, provider.PartToolCall, parts[4].Type)
		assert.Equal(t, `{"q":"test"}`, parts[4].Input)
		assert.Equal(t, "call_1", parts[4].ToolCallID)
		assert.Equal(t, "search", parts[4].ToolName)
	})
}

func TestStreamAdapter_EmptyInputJsonDelta(t *testing.T) {
	events := []anthropic.BetaRawMessageStreamEventUnion{
		unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"no_args"}}`),
		unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`),
		unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
	}

	parts := collectParts(events)

	for _, p := range parts {
		if p.Type == provider.PartToolInputDelta {
			t.Fatalf("expected no PartToolInputDelta for empty partial_json, got delta=%q", p.Delta)
		}
	}

	require.Len(t, parts, 3)
	assert.Equal(t, provider.PartToolInputStart, parts[0].Type)
	assert.Equal(t, provider.PartToolInputEnd, parts[1].Type)
	assert.Equal(t, provider.PartToolCall, parts[2].Type)
	assert.Equal(t, "{}", parts[2].Input)
}

func TestStreamAdapter_MessageEvents(t *testing.T) {
	t.Run("message_start", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_123","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":0}}}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartResponseMeta, parts[0].Type)
		assert.Equal(t, "msg_123", parts[0].ResponseID)
		assert.Equal(t, "claude-sonnet-4-6", parts[0].ModelID)
	})

	t.Run("message_start carries provider", func(t *testing.T) {
		adapter := &streamAdapter{
			blocks:          make(map[int64]*blockState),
			serverToolCalls: make(map[string]string),
			mcpToolCalls:    make(map[string]mcpToolCallInfo),
			generateID:      defaultGenerateID,
			providerName:    "anthropic.vertex",
		}
		ch := make(chan provider.StreamPart, 8)
		require.NoError(t, adapter.handleEvent(
			unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_123","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":0}}}`),
			ch,
		))
		close(ch)

		part := <-ch
		assert.Equal(t, provider.PartResponseMeta, part.Type)
		assert.Equal(t, "anthropic.vertex", part.Provider)
	})

	t.Run("message_delta", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":50}}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartFinish, parts[0].Type)
		require.NotNil(t, parts[0].FinishReason)
		assert.Equal(t, provider.FinishReasonStop, parts[0].FinishReason.Unified)
	})

	t.Run("duplicate open message start", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":0}}}`),
			unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":0}}}`),
		}

		parts := collectParts(events)
		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartResponseMeta, parts[0].Type)
	})

	t.Run("different message starts while open", func(t *testing.T) {
		adapter := &streamAdapter{blocks: make(map[int64]*blockState)}
		ch := make(chan provider.StreamPart, 8)
		require.NoError(t, adapter.handleEvent(
			unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":0}}}`),
			ch,
		))
		err := adapter.handleEvent(
			unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_2","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":0}}}`),
			ch,
		)
		require.ErrorContains(t, err, `message "msg_2" while message "msg_1" is still open`)
		require.NoError(t, adapter.handleEvent(
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"ignored"}}`),
			ch,
		))
		close(ch)

		parts := make([]provider.StreamPart, 0, len(ch))
		for part := range ch {
			parts = append(parts, part)
		}
		require.Len(t, parts, 1)
		assert.Equal(t, "msg_1", parts[0].ResponseID)
	})
}

func TestStreamAdapter_CacheMetrics(t *testing.T) {
	t.Run("message_start_with_cache", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":0,"cache_creation_input_tokens":50,"cache_read_input_tokens":30}}}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 1)
		require.NotNil(t, parts[0].Usage)
		require.NotNil(t, parts[0].Usage.InputTokens.Total)
		assert.Equal(t, 180, *parts[0].Usage.InputTokens.Total)
		require.NotNil(t, parts[0].Usage.InputTokens.NoCache)
		assert.Equal(t, 100, *parts[0].Usage.InputTokens.NoCache)
		require.NotNil(t, parts[0].Usage.InputTokens.CacheRead)
		assert.Equal(t, 30, *parts[0].Usage.InputTokens.CacheRead)
		require.NotNil(t, parts[0].Usage.InputTokens.CacheWrite)
		assert.Equal(t, 50, *parts[0].Usage.InputTokens.CacheWrite)
		assert.JSONEq(t, `{"input_tokens":100,"output_tokens":0,"cache_creation_input_tokens":50,"cache_read_input_tokens":30}`, string(parts[0].Usage.Raw))
	})

	t.Run("message_start_no_cache", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`),
		}

		parts := collectParts(events)

		require.NotNil(t, parts[0].Usage.InputTokens.CacheRead)
		assert.Equal(t, 0, *parts[0].Usage.InputTokens.CacheRead)
	})

	t.Run("message_delta_with_cache", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":75}}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 1)
		require.NotNil(t, parts[0].Usage)
		require.NotNil(t, parts[0].Usage.InputTokens.CacheRead)
		assert.Equal(t, 75, *parts[0].Usage.InputTokens.CacheRead)
		require.NotNil(t, parts[0].Usage.InputTokens.CacheWrite)
		assert.Equal(t, 0, *parts[0].Usage.InputTokens.CacheWrite)
	})

	t.Run("message_delta_no_cache", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`),
		}

		parts := collectParts(events)

		require.NotNil(t, parts[0].Usage.InputTokens.CacheRead)
		assert.Equal(t, 0, *parts[0].Usage.InputTokens.CacheRead)
	})
}

func TestStreamAdapter_ServerToolUse(t *testing.T) {
	t.Run("lifecycle", func(t *testing.T) {
		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.web_search_20250305",
			Name: "search_docs",
		}})
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"web_search"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"test\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectPartsWithMapping(events, mapping)

		require.Len(t, parts, 5)

		assert.Equal(t, provider.PartToolInputStart, parts[0].Type)
		assert.True(t, parts[0].ProviderExecuted, "part[0] should have ProviderExecuted=true")
		assert.Equal(t, "stu_1", parts[0].ID)
		assert.Equal(t, "search_docs", parts[0].ToolName)

		assert.Equal(t, provider.PartToolInputDelta, parts[1].Type)

		assert.Equal(t, provider.PartToolInputEnd, parts[3].Type)

		assert.Equal(t, provider.PartToolCall, parts[4].Type)
		assert.True(t, parts[4].ProviderExecuted, "part[4] (tool-call) should have ProviderExecuted=true")
		assert.Equal(t, "stu_1", parts[4].ToolCallID)
		assert.Equal(t, "search_docs", parts[4].ToolName)
		assert.Equal(t, `{"query":"test"}`, parts[4].Input)
	})

	t.Run("unknown_name", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_2","name":"future_tool"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 4)
		assert.Equal(t, provider.PartToolInputStart, parts[0].Type)
		assert.Equal(t, "future_tool", parts[0].ToolName)
		assert.Equal(t, provider.PartToolCall, parts[3].Type)
		assert.Equal(t, "future_tool", parts[3].ToolName)
		assert.True(t, parts[3].ProviderExecuted)
	})

	t.Run("regular_tool_not_provider_executed", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"search"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		for i, p := range parts {
			assert.False(t, p.ProviderExecuted, "part[%d] should NOT have ProviderExecuted=true for regular tool_use", i)
		}
	})
}

func TestStreamAdapter_CallerMetadata(t *testing.T) {
	t.Run("direct_caller", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"search","caller":{"type":"direct"}}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var toolCall provider.StreamPart
		for _, p := range parts {
			if p.Type == provider.PartToolCall {
				toolCall = p
			}
		}

		require.NotNil(t, toolCall.ProviderMetadata)
		raw, ok := toolCall.ProviderMetadata["anthropic"]
		require.True(t, ok)
		var meta map[string]any
		require.NoError(t, json.Unmarshal(raw, &meta))
		caller, ok := meta["caller"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "direct", caller["type"])
		assert.Nil(t, caller["toolId"])
	})

	t.Run("code_execution_caller_with_tool_id", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_2","name":"query_db","caller":{"type":"code_execution_20250825","tool_id":"toolu_123"}}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var toolCall provider.StreamPart
		for _, p := range parts {
			if p.Type == provider.PartToolCall {
				toolCall = p
			}
		}

		require.NotNil(t, toolCall.ProviderMetadata)
		raw := toolCall.ProviderMetadata["anthropic"]
		var meta map[string]any
		require.NoError(t, json.Unmarshal(raw, &meta))
		caller, ok := meta["caller"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "code_execution_20250825", caller["type"])
		assert.Equal(t, "toolu_123", caller["toolId"])
	})

	t.Run("no_caller", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_3","name":"weather"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var toolCall provider.StreamPart
		for _, p := range parts {
			if p.Type == provider.PartToolCall {
				toolCall = p
			}
		}

		assert.Nil(t, toolCall.ProviderMetadata)
	})
}

func TestStreamAdapter_ToolResults(t *testing.T) {
	t.Run("web_search_success", func(t *testing.T) {
		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.web_search_20250305",
			Name: "search_docs",
		}})
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"web_search_tool_result","tool_use_id":"stu_1","content":[{"type":"web_search_result","title":"Test Page","url":"https://example.com","page_age":"2d","encrypted_content":"abc"},{"type":"web_search_result","title":"Other Page","url":"https://other.com","page_age":"1w","encrypted_content":"def"}]}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}

		parts := collectPartsWithMapping(events, mapping)

		require.GreaterOrEqual(t, len(parts), 3, "expected at least 3 parts (1 tool-result + 2 sources)")

		assert.Equal(t, provider.PartToolResult, parts[0].Type)
		assert.Equal(t, "stu_1", parts[0].ToolCallID)
		assert.Equal(t, "search_docs", parts[0].ToolName)
		assert.True(t, parts[0].ProviderExecuted, "part[0] should have ProviderExecuted=true")

		assert.Equal(t, provider.PartSource, parts[1].Type)
		require.NotNil(t, parts[1].Source)
		assert.Equal(t, provider.SourceTypeURL, parts[1].Source.SourceType)
		assert.Equal(t, "https://example.com", parts[1].Source.URL)
		assert.Equal(t, "Test Page", parts[1].Source.Title)
		assert.NotEmpty(t, parts[1].Source.ID, "streaming web search source should have non-empty ID")

		assert.Equal(t, provider.PartSource, parts[2].Type)
		assert.Equal(t, "https://other.com", parts[2].Source.URL)
		assert.NotEmpty(t, parts[2].Source.ID, "streaming web search source should have non-empty ID")
	})

	t.Run("web_search_error", func(t *testing.T) {
		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.web_search_20250305",
			Name: "search_docs",
		}})
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"web_search_tool_result","tool_use_id":"stu_1","content":{"type":"web_search_tool_result_error","error_code":"max_uses_exceeded"}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}

		parts := collectPartsWithMapping(events, mapping)

		require.GreaterOrEqual(t, len(parts), 1)

		assert.Equal(t, provider.PartToolResult, parts[0].Type)
		assert.Equal(t, "search_docs", parts[0].ToolName)
		assert.True(t, parts[0].ProviderExecuted, "part[0] should have ProviderExecuted=true")

		for _, p := range parts {
			assert.False(t, p.Type == provider.PartSource, "error result should NOT emit source parts")
		}
	})

	t.Run("web_fetch_success_text", func(t *testing.T) {
		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.web_fetch_20250910",
			Name: "fetch_page",
		}})
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"web_fetch_tool_result","tool_use_id":"stu_1","content":{"type":"web_fetch_result","url":"https://example.com","retrieved_at":"2025-01-01T00:00:00Z","content":{"type":"document","title":"Example Page","citations":{"enabled":true},"source":{"type":"text","media_type":"text/plain","data":"Hello world"}}}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}

		parts := collectPartsWithMapping(events, mapping)

		require.GreaterOrEqual(t, len(parts), 1)
		assert.Equal(t, provider.PartToolResult, parts[0].Type)
		assert.Equal(t, "stu_1", parts[0].ToolCallID)
		assert.Equal(t, "fetch_page", parts[0].ToolName)
		assert.True(t, parts[0].ProviderExecuted)
		assert.False(t, parts[0].IsError)

		var result map[string]any
		require.NoError(t, json.Unmarshal(parts[0].Result, &result))
		assert.Equal(t, "web_fetch_result", result["type"])
		assert.Equal(t, "https://example.com", result["url"])
		assert.Equal(t, "2025-01-01T00:00:00Z", result["retrievedAt"])
		content := result["content"].(map[string]any)
		assert.Equal(t, "document", content["type"])
		assert.Equal(t, "Example Page", content["title"])
		source := content["source"].(map[string]any)
		assert.Equal(t, "text", source["type"])
		assert.Equal(t, "text/plain", source["mediaType"])
		assert.Equal(t, "Hello world", source["data"])
	})

	t.Run("web_fetch_success_pdf", func(t *testing.T) {
		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.web_fetch_20250910",
			Name: "fetch_page",
		}})
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"web_fetch_tool_result","tool_use_id":"stu_2","content":{"type":"web_fetch_result","url":"https://example.com/doc.pdf","retrieved_at":"2025-06-01T12:00:00Z","content":{"type":"document","title":"Report","citations":{"enabled":true},"source":{"type":"base64","media_type":"application/pdf","data":"JVBER..."}}}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}

		parts := collectPartsWithMapping(events, mapping)

		require.GreaterOrEqual(t, len(parts), 1)
		var result map[string]any
		require.NoError(t, json.Unmarshal(parts[0].Result, &result))
		content := result["content"].(map[string]any)
		source := content["source"].(map[string]any)
		assert.Equal(t, "base64", source["type"])
		assert.Equal(t, "application/pdf", source["mediaType"])
	})

	t.Run("web_fetch_success_nil_title_falls_back_to_url", func(t *testing.T) {
		adapter := &streamAdapter{
			blocks:            make(map[int64]*blockState),
			mapping:           toolNameMapping{},
			serverToolCalls:   make(map[string]string),
			mcpToolCalls:      make(map[string]mcpToolCallInfo),
			citationDocuments: nil,
			generateID:        defaultGenerateID,
		}
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"web_fetch_tool_result","tool_use_id":"stu_3","content":{"type":"web_fetch_result","url":"https://notitle.com","retrieved_at":"2025-01-01T00:00:00Z","content":{"type":"document","title":"","citations":{"enabled":false},"source":{"type":"text","media_type":"text/plain","data":"content"}}}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}
		ch := make(chan provider.StreamPart, 100)
		for _, e := range events {
			_ = adapter.handleEvent(e, ch)
		}
		close(ch)

		require.Len(t, adapter.citationDocuments, 1)
		assert.Equal(t, "https://notitle.com", adapter.citationDocuments[0].title)
		assert.Equal(t, "text/plain", adapter.citationDocuments[0].mediaType)
	})

	t.Run("web_fetch_error", func(t *testing.T) {
		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.web_fetch_20250910",
			Name: "fetch_page",
		}})
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"web_fetch_tool_result","tool_use_id":"stu_4","content":{"type":"web_fetch_tool_result_error","error_code":"too_many_requests"}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}

		parts := collectPartsWithMapping(events, mapping)

		require.GreaterOrEqual(t, len(parts), 1)
		assert.Equal(t, provider.PartToolResult, parts[0].Type)
		assert.Equal(t, "fetch_page", parts[0].ToolName)
		assert.True(t, parts[0].IsError)
		assert.True(t, parts[0].ProviderExecuted)

		var errResult map[string]string
		require.NoError(t, json.Unmarshal(parts[0].Result, &errResult))
		assert.Equal(t, "web_fetch_tool_result_error", errResult["type"])
		assert.Equal(t, "too_many_requests", errResult["errorCode"])
	})

	t.Run("web_fetch_success_pushes_citation_document", func(t *testing.T) {
		adapter := &streamAdapter{
			blocks:            make(map[int64]*blockState),
			mapping:           toolNameMapping{},
			serverToolCalls:   make(map[string]string),
			mcpToolCalls:      make(map[string]mcpToolCallInfo),
			citationDocuments: nil,
			generateID:        defaultGenerateID,
		}
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"web_fetch_tool_result","tool_use_id":"stu_5","content":{"type":"web_fetch_result","url":"https://example.com","retrieved_at":"2025-01-01T00:00:00Z","content":{"type":"document","title":"My Doc","citations":{"enabled":true},"source":{"type":"text","media_type":"text/plain","data":"text"}}}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}
		ch := make(chan provider.StreamPart, 100)
		for _, e := range events {
			_ = adapter.handleEvent(e, ch)
		}
		close(ch)

		require.Len(t, adapter.citationDocuments, 1)
		assert.Equal(t, "My Doc", adapter.citationDocuments[0].title)
		assert.Equal(t, "text/plain", adapter.citationDocuments[0].mediaType)
	})

	t.Run("web_fetch_error_does_not_push_citation_document", func(t *testing.T) {
		adapter := &streamAdapter{
			blocks:            make(map[int64]*blockState),
			mapping:           toolNameMapping{},
			serverToolCalls:   make(map[string]string),
			mcpToolCalls:      make(map[string]mcpToolCallInfo),
			citationDocuments: nil,
			generateID:        defaultGenerateID,
		}
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"web_fetch_tool_result","tool_use_id":"stu_6","content":{"type":"web_fetch_tool_result_error","error_code":"invalid_tool_input"}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}
		ch := make(chan provider.StreamPart, 100)
		for _, e := range events {
			_ = adapter.handleEvent(e, ch)
		}
		close(ch)

		assert.Empty(t, adapter.citationDocuments)
	})

	t.Run("tool_search_success", func(t *testing.T) {
		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.tool_search_bm25_20251119",
			Name: "search_tools",
		}})
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_2","name":"tool_search_tool_bm25"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"tool_search_tool_result","tool_use_id":"stu_2","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","name":"my_func","description":"A function"}]}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}

		parts := collectPartsWithMapping(events, mapping)

		require.GreaterOrEqual(t, len(parts), 5)

		toolResult := parts[len(parts)-1]
		assert.Equal(t, provider.PartToolResult, toolResult.Type)
		assert.Equal(t, "stu_2", toolResult.ToolCallID)
		assert.Equal(t, "search_tools", toolResult.ToolName)
		assert.True(t, toolResult.ProviderExecuted, "tool result should have ProviderExecuted=true")
	})

	t.Run("tool_search_error", func(t *testing.T) {
		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.tool_search_regex_20251119",
			Name: "search_regex",
		}})
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"tool_search_tool_result","tool_use_id":"stu_2","content":{"type":"tool_search_tool_result_error","error_code":"internal_error","error_message":"search failed"}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}

		parts := collectPartsWithMapping(events, mapping)

		require.GreaterOrEqual(t, len(parts), 1)

		assert.Equal(t, provider.PartToolResult, parts[0].Type)
		assert.Equal(t, "search_regex", parts[0].ToolName)
		assert.True(t, parts[0].ProviderExecuted, "part[0] should have ProviderExecuted=true")
	})
}

func TestStreamAdapter_MCPToolUse(t *testing.T) {
	t.Run("emits_PartToolCall_directly", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"mcp_tool_use","id":"tc_123","name":"get_weather","server_name":"weather-server","input":{"city":"London"}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 1)
		p := parts[0]
		assert.Equal(t, provider.PartToolCall, p.Type)
		assert.Equal(t, "tc_123", p.ToolCallID)
		assert.Equal(t, "get_weather", p.ToolName)
		assert.Equal(t, `{"city":"London"}`, p.Input)
		assert.True(t, p.ProviderExecuted)
		assert.Equal(t, boolPtr(true), p.Dynamic)

		require.NotNil(t, p.ProviderMetadata)
		raw, ok := p.ProviderMetadata["anthropic"]
		require.True(t, ok)
		var meta map[string]string
		require.NoError(t, json.Unmarshal(raw, &meta))
		assert.Equal(t, "mcp-tool-use", meta["type"])
		assert.Equal(t, "weather-server", meta["serverName"])
	})

	t.Run("no_PartToolInputStart_emitted", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"mcp_tool_use","id":"tc_1","name":"tool_a","server_name":"srv","input":{}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		for _, p := range parts {
			assert.NotEqual(t, provider.PartToolInputStart, p.Type)
			assert.NotEqual(t, provider.PartToolInputDelta, p.Type)
			assert.NotEqual(t, provider.PartToolInputEnd, p.Type)
		}
	})
}

func TestStreamAdapter_MCPToolResult(t *testing.T) {
	t.Run("success_string_content", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"mcp_tool_use","id":"tc_123","name":"get_weather","server_name":"weather-server","input":{"city":"London"}}}`),
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"mcp_tool_result","tool_use_id":"tc_123","is_error":false,"content":"Weather is sunny"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 2)
		p := parts[1]
		assert.Equal(t, provider.PartToolResult, p.Type)
		assert.Equal(t, "tc_123", p.ToolCallID)
		assert.Equal(t, "get_weather", p.ToolName)
		assert.Equal(t, boolPtr(true), p.Dynamic)
		assert.False(t, p.IsError)
		assert.JSONEq(t, `"Weather is sunny"`, string(p.Result))

		require.NotNil(t, p.ProviderMetadata)
		raw := p.ProviderMetadata["anthropic"]
		var meta map[string]string
		require.NoError(t, json.Unmarshal(raw, &meta))
		assert.Equal(t, "mcp-tool-use", meta["type"])
		assert.Equal(t, "weather-server", meta["serverName"])
	})

	t.Run("success_array_content", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"mcp_tool_use","id":"tc_123","name":"echo","server_name":"echo","input":{}}}`),
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"mcp_tool_result","tool_use_id":"tc_123","is_error":false,"content":[{"type":"text","text":"Tool echo: hello world"}]}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 2)
		p := parts[1]
		assert.Equal(t, provider.PartToolResult, p.Type)
		assert.Equal(t, "tc_123", p.ToolCallID)
		assert.Equal(t, "echo", p.ToolName)
		assert.Equal(t, boolPtr(true), p.Dynamic)
		assert.False(t, p.IsError)
		assert.JSONEq(t, `[{"type":"text","text":"Tool echo: hello world"}]`, string(p.Result))
	})

	t.Run("error", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"mcp_tool_use","id":"tc_456","name":"fail_tool","server_name":"srv","input":{}}}`),
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"mcp_tool_result","tool_use_id":"tc_456","is_error":true,"content":"Tool execution failed"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 2)
		p := parts[1]
		assert.Equal(t, provider.PartToolResult, p.Type)
		assert.True(t, p.IsError)
		assert.JSONEq(t, `"Tool execution failed"`, string(p.Result))
		assert.Equal(t, boolPtr(true), p.Dynamic)
	})

	t.Run("missing_tracking_entry", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"mcp_tool_result","tool_use_id":"orphan_id","is_error":false,"content":"some result"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		require.Len(t, parts, 1)
		p := parts[0]
		assert.Equal(t, provider.PartToolResult, p.Type)
		assert.Equal(t, "orphan_id", p.ToolCallID)
		assert.Empty(t, p.ToolName)
		assert.Nil(t, p.ProviderMetadata)
		assert.Equal(t, boolPtr(true), p.Dynamic)
	})
}

func TestStreamAdapter_MCPFullSequence(t *testing.T) {
	events := []anthropic.BetaRawMessageStreamEventUnion{
		unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_mcp","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":0}}}`),

		unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Checking weather."}}`),
		unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),

		unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"mcp_tool_use","id":"tc_w1","name":"get_weather","server_name":"weather-srv","input":{"city":"SF"}}}`),
		unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),

		unmarshalEvent(t, `{"type":"content_block_start","index":2,"content_block":{"type":"mcp_tool_result","tool_use_id":"tc_w1","is_error":false,"content":"Sunny, 72F"}}`),
		unmarshalEvent(t, `{"type":"content_block_stop","index":2}`),

		unmarshalEvent(t, `{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"call_1","name":"user_func"}}`),
		unmarshalEvent(t, `{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"x\":1}"}}`),
		unmarshalEvent(t, `{"type":"content_block_stop","index":3}`),

		unmarshalEvent(t, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":10,"output_tokens":100}}`),
	}

	parts := collectParts(events)

	var mcpToolCalls, mcpToolResults, regularToolCalls, textParts int
	for _, p := range parts {
		isDyn := p.Dynamic != nil && *p.Dynamic
		switch {
		case p.Type == provider.PartToolCall && isDyn:
			mcpToolCalls++
			assert.True(t, p.ProviderExecuted)
		case p.Type == provider.PartToolResult && isDyn:
			mcpToolResults++
		case p.Type == provider.PartToolCall && !isDyn:
			regularToolCalls++
		case p.Type == provider.PartTextStart || p.Type == provider.PartTextDelta:
			textParts++
		}
	}

	assert.Equal(t, 1, mcpToolCalls, "expected 1 MCP tool call")
	assert.Equal(t, 1, mcpToolResults, "expected 1 MCP tool result")
	assert.Equal(t, 1, regularToolCalls, "expected 1 regular tool call")
	assert.Greater(t, textParts, 0, "expected text parts")
}

func TestStreamAdapter_MixedToolUseAndServerToolUse(t *testing.T) {
	events := []anthropic.BetaRawMessageStreamEventUnion{
		unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":0}}}`),

		unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me search for that."}}`),
		unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),

		unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"server_tool_use","id":"stu_1","name":"web_search"}}`),
		unmarshalEvent(t, `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"go concurrency\"}"}}`),
		unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),

		unmarshalEvent(t, `{"type":"content_block_start","index":2,"content_block":{"type":"web_search_tool_result","tool_use_id":"stu_1","content":[{"type":"web_search_result","title":"Go Concurrency","url":"https://go.dev/blog/concurrency","page_age":"1y","encrypted_content":"enc1"}]}}`),
		unmarshalEvent(t, `{"type":"content_block_stop","index":2}`),

		unmarshalEvent(t, `{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"call_1","name":"user_func"}}`),
		unmarshalEvent(t, `{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"x\":1}"}}`),
		unmarshalEvent(t, `{"type":"content_block_stop","index":3}`),

		unmarshalEvent(t, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":10,"output_tokens":100}}`),
	}

	parts := collectParts(events)

	var serverToolParts, regularToolParts, sourceParts int
	for _, p := range parts {
		switch {
		case p.Type == provider.PartToolCall && p.ProviderExecuted:
			serverToolParts++
		case p.Type == provider.PartToolCall && !p.ProviderExecuted:
			regularToolParts++
		case p.Type == provider.PartSource:
			sourceParts++
		}
	}

	assert.Equal(t, 1, serverToolParts, "expected 1 server tool call")
	assert.Equal(t, 1, regularToolParts, "expected 1 regular tool call")
	assert.Equal(t, 1, sourceParts, "expected 1 source part")

	hasTextStart := false
	hasToolResult := false
	hasFinish := false
	for _, p := range parts {
		if p.Type == provider.PartTextStart {
			hasTextStart = true
		}
		if p.Type == provider.PartToolResult {
			hasToolResult = true
		}
		if p.Type == provider.PartFinish {
			hasFinish = true
		}
	}
	assert.True(t, hasTextStart, "missing text-start part")
	assert.True(t, hasToolResult, "missing tool-result part")
	assert.True(t, hasFinish, "missing finish part")
}

func TestStreamAdapter_CitationsDelta(t *testing.T) {
	textBlockStart := unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)

	t.Run("web search result location", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			textBlockStart,
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"citations_delta","citation":{"type":"web_search_result_location","url":"https://example.com","title":"Example","cited_text":"some text","encrypted_index":"enc123"}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}
		parts := collectPartsWithOpts(events, toolNameMapping{}, nil, seqIDGenerator())
		require.Len(t, parts, 3)
		assert.Equal(t, provider.PartSource, parts[1].Type)
		require.NotNil(t, parts[1].Source)
		assert.Equal(t, provider.SourceTypeURL, parts[1].Source.SourceType)
		assert.Equal(t, "https://example.com", parts[1].Source.URL)
		assert.Equal(t, "Example", parts[1].Source.Title)
		assert.NotEmpty(t, parts[1].Source.ID)

		var meta map[string]any
		require.NoError(t, json.Unmarshal(parts[1].Source.ProviderMetadata["anthropic"], &meta))
		assert.Equal(t, "some text", meta["citedText"])
		assert.Equal(t, "enc123", meta["encryptedIndex"])

		assert.Equal(t, provider.PartTextEnd, parts[2].Type)
		assert.JSONEq(t, `{"citations":[{"type":"web_search_result_location","url":"https://example.com","title":"Example","cited_text":"some text","encrypted_index":"enc123"}]}`, string(parts[2].ProviderMetadata["anthropic"]))
	})

	t.Run("page location with tracked document", func(t *testing.T) {
		docs := []citationDocument{
			{title: "Report", filename: "report.pdf", mediaType: "application/pdf"},
		}
		events := []anthropic.BetaRawMessageStreamEventUnion{
			textBlockStart,
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"citations_delta","citation":{"type":"page_location","document_index":0,"cited_text":"page text","start_page_number":1,"end_page_number":3,"document_title":"","file_id":"f1"}}}`),
		}
		parts := collectPartsWithOpts(events, toolNameMapping{}, docs, seqIDGenerator())
		require.Len(t, parts, 2)
		assert.Equal(t, provider.PartSource, parts[1].Type)
		src := parts[1].Source
		require.NotNil(t, src)
		assert.Equal(t, provider.SourceTypeDocument, src.SourceType)
		assert.Equal(t, "application/pdf", src.MediaType)
		assert.Equal(t, "Report", src.Title)
		assert.Equal(t, "report.pdf", src.Filename)

		var meta map[string]any
		require.NoError(t, json.Unmarshal(src.ProviderMetadata["anthropic"], &meta))
		assert.Equal(t, "page text", meta["citedText"])
		assert.Equal(t, float64(1), meta["startPageNumber"])
		assert.Equal(t, float64(3), meta["endPageNumber"])
	})

	t.Run("char location with tracked document", func(t *testing.T) {
		docs := []citationDocument{
			{title: "Notes", filename: "notes.txt", mediaType: "text/plain"},
		}
		events := []anthropic.BetaRawMessageStreamEventUnion{
			textBlockStart,
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"citations_delta","citation":{"type":"char_location","document_index":0,"cited_text":"char text","start_char_index":10,"end_char_index":50,"document_title":"","file_id":"f1"}}}`),
		}
		parts := collectPartsWithOpts(events, toolNameMapping{}, docs, seqIDGenerator())
		require.Len(t, parts, 2)
		src := parts[1].Source
		require.NotNil(t, src)
		assert.Equal(t, provider.SourceTypeDocument, src.SourceType)
		assert.Equal(t, "text/plain", src.MediaType)

		var meta map[string]any
		require.NoError(t, json.Unmarshal(src.ProviderMetadata["anthropic"], &meta))
		assert.Equal(t, float64(10), meta["startCharIndex"])
		assert.Equal(t, float64(50), meta["endCharIndex"])
	})

	t.Run("unknown citation type produces no output", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			textBlockStart,
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"citations_delta","citation":{"type":"content_block_location","document_index":0,"cited_text":"text","start_block_index":0,"end_block_index":1}}}`),
		}
		parts := collectPartsWithOpts(events, toolNameMapping{}, nil, seqIDGenerator())
		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartTextStart, parts[0].Type)
	})

	t.Run("out-of-range document index produces no output", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			textBlockStart,
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"citations_delta","citation":{"type":"page_location","document_index":99,"cited_text":"text","start_page_number":1,"end_page_number":1,"document_title":"","file_id":"f1"}}}`),
		}
		parts := collectPartsWithOpts(events, toolNameMapping{}, nil, seqIDGenerator())
		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartTextStart, parts[0].Type)
	})
}

func collectPartsWithJsonResponseTool(events []anthropic.BetaRawMessageStreamEventUnion) []provider.StreamPart {
	adapter := &streamAdapter{
		blocks:               make(map[int64]*blockState),
		mapping:              toolNameMapping{},
		serverToolCalls:      make(map[string]string),
		mcpToolCalls:         make(map[string]mcpToolCallInfo),
		usesJsonResponseTool: true,
		generateID:           defaultGenerateID,
	}
	ch := make(chan provider.StreamPart, 100)
	for _, e := range events {
		_ = adapter.handleEvent(e, ch)
	}
	close(ch)

	var parts []provider.StreamPart
	for p := range ch {
		parts = append(parts, p)
	}
	return parts
}

func TestStreamAdapter_JsonResponseTool(t *testing.T) {
	t.Run("tool_input_remapped_to_text_block", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_json","name":"json"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"name\":"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Alice\",\"age\":30}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectPartsWithJsonResponseTool(events)

		for _, p := range parts {
			assert.NotEqual(t, provider.PartToolInputStart, p.Type, "should not emit PartToolInputStart for json tool")
			assert.NotEqual(t, provider.PartToolInputDelta, p.Type, "should not emit PartToolInputDelta for json tool")
			assert.NotEqual(t, provider.PartToolInputEnd, p.Type, "should not emit PartToolInputEnd for json tool")
			assert.NotEqual(t, provider.PartToolCall, p.Type, "should not emit PartToolCall for json tool")
		}

		require.Len(t, parts, 4)
		assert.Equal(t, provider.PartTextStart, parts[0].Type)
		assert.Equal(t, provider.PartTextDelta, parts[1].Type)
		assert.Equal(t, `{"name":`, parts[1].Delta)
		assert.Equal(t, provider.PartTextDelta, parts[2].Type)
		assert.Equal(t, `"Alice","age":30}`, parts[2].Delta)
		assert.Equal(t, provider.PartTextEnd, parts[3].Type)
	})

	t.Run("finish_reason_remapped_from_tool_use_to_stop", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_json","name":"json"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
			unmarshalEvent(t, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":10,"output_tokens":50}}`),
		}

		parts := collectPartsWithJsonResponseTool(events)

		var finishPart *provider.StreamPart
		for i := range parts {
			if parts[i].Type == provider.PartFinish {
				finishPart = &parts[i]
			}
		}
		require.NotNil(t, finishPart, "expected a PartFinish")
		assert.Equal(t, provider.FinishReasonStop, finishPart.FinishReason.Unified, "finish reason should be remapped to stop")
		assert.Equal(t, "tool_use", finishPart.FinishReason.Raw, "raw finish reason should preserve original")
	})

	t.Run("non_tool_use_finish_reason_preserved", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":50}}`),
		}

		parts := collectPartsWithJsonResponseTool(events)

		require.Len(t, parts, 1)
		assert.Equal(t, provider.FinishReasonStop, parts[0].FinishReason.Unified, "end_turn should remain stop")
		assert.Equal(t, "end_turn", parts[0].FinishReason.Raw)
	})

	t.Run("regular_tools_unaffected", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_search","name":"search"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"test\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectPartsWithJsonResponseTool(events)

		var hasToolInputStart, hasToolInputDelta, hasToolInputEnd, hasToolCall bool
		for _, p := range parts {
			switch p.Type {
			case provider.PartToolInputStart:
				hasToolInputStart = true
				assert.Equal(t, "search", p.ToolName)
			case provider.PartToolInputDelta:
				hasToolInputDelta = true
			case provider.PartToolInputEnd:
				hasToolInputEnd = true
			case provider.PartToolCall:
				hasToolCall = true
				assert.Equal(t, "search", p.ToolName)
			}
		}
		assert.True(t, hasToolInputStart, "regular tool should emit PartToolInputStart")
		assert.True(t, hasToolInputDelta, "regular tool should emit PartToolInputDelta")
		assert.True(t, hasToolInputEnd, "regular tool should emit PartToolInputEnd")
		assert.True(t, hasToolCall, "regular tool should emit PartToolCall")
	})

	t.Run("text_blocks_suppressed_json_tool_emitted", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Thinking..."}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_json","name":"json"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"result\":42}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
			unmarshalEvent(t, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":10,"output_tokens":50}}`),
		}

		parts := collectPartsWithJsonResponseTool(events)

		var regularTextDeltas, jsonTextDeltas int
		for _, p := range parts {
			if p.Type == provider.PartTextDelta {
				if p.Delta == "Thinking..." {
					regularTextDeltas++
				} else {
					jsonTextDeltas++
				}
			}
		}
		assert.Equal(t, 0, regularTextDeltas, "regular text blocks should be suppressed")
		assert.Equal(t, 1, jsonTextDeltas, "expected 1 json-remapped text delta")
	})

	t.Run("finish_reason_not_remapped_when_user_tool_called", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_search","name":"search"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
			unmarshalEvent(t, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":10,"output_tokens":50}}`),
		}

		parts := collectPartsWithJsonResponseTool(events)

		var finishPart *provider.StreamPart
		for i := range parts {
			if parts[i].Type == provider.PartFinish {
				finishPart = &parts[i]
			}
		}
		require.NotNil(t, finishPart, "expected a PartFinish")
		assert.Equal(t, provider.FinishReasonToolCalls, finishPart.FinishReason.Unified,
			"when model calls a user tool (not json), finish reason should remain tool-calls")
	})

	t.Run("flag_disabled_no_remapping", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_json","name":"json"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var hasToolInputStart, hasToolCall bool
		for _, p := range parts {
			switch p.Type {
			case provider.PartToolInputStart:
				hasToolInputStart = true
			case provider.PartToolCall:
				hasToolCall = true
			}
		}
		assert.True(t, hasToolInputStart, "without flag, json tool should be treated as regular tool")
		assert.True(t, hasToolCall, "without flag, json tool should emit PartToolCall")
	})
}

func TestStreamAdapter_CodeExecutionDeltaRewriting(t *testing.T) {
	t.Run("bash_code_execution_first_delta_rewritten", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"bash_code_execution"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"code\": \"ls -la\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.code_execution_20250825",
			Name: "code_exec",
		}})
		parts := collectPartsWithMapping(events, mapping)

		var inputDelta, toolCall *provider.StreamPart
		for i := range parts {
			if parts[i].Type == provider.PartToolInputDelta {
				inputDelta = &parts[i]
			}
			if parts[i].Type == provider.PartToolCall {
				toolCall = &parts[i]
			}
		}
		require.NotNil(t, inputDelta, "expected PartToolInputDelta")
		assert.Contains(t, inputDelta.Delta, `"type": "bash_code_execution"`)
		assert.Contains(t, inputDelta.Delta, `"code": "ls -la"`)

		require.NotNil(t, toolCall, "expected PartToolCall")
		assert.Equal(t, "code_exec", toolCall.ToolName)
		assert.True(t, toolCall.ProviderExecuted)
	})

	t.Run("text_editor_code_execution_first_delta_rewritten", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"text_editor_code_execution"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\": \"view\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var inputDelta *provider.StreamPart
		for i := range parts {
			if parts[i].Type == provider.PartToolInputDelta {
				inputDelta = &parts[i]
			}
		}
		require.NotNil(t, inputDelta)
		assert.Contains(t, inputDelta.Delta, `"type": "text_editor_code_execution"`)
	})

	t.Run("empty_deltas_skipped_before_rewrite", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"bash_code_execution"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"code\": \"echo hi\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var inputDeltas []provider.StreamPart
		for _, p := range parts {
			if p.Type == provider.PartToolInputDelta {
				inputDeltas = append(inputDeltas, p)
			}
		}
		require.Len(t, inputDeltas, 1, "empty delta should not be emitted")
		assert.Contains(t, inputDeltas[0].Delta, `"type": "bash_code_execution"`)
	})

	t.Run("subsequent_deltas_not_rewritten", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"bash_code_execution"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"code\":"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":" \"ls\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var inputDeltas []provider.StreamPart
		for _, p := range parts {
			if p.Type == provider.PartToolInputDelta {
				inputDeltas = append(inputDeltas, p)
			}
		}
		require.Len(t, inputDeltas, 2)
		assert.Contains(t, inputDeltas[0].Delta, `"type": "bash_code_execution"`)
		assert.Equal(t, ` "ls"}`, inputDeltas[1].Delta)
	})
}

func TestStreamAdapter_ProgrammaticToolCallInjection(t *testing.T) {
	t.Run("code_field_without_type_injects_programmatic", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"code_execution"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"code\": \"print('hi')\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var inputDelta, toolCall *provider.StreamPart
		for i := range parts {
			if parts[i].Type == provider.PartToolInputDelta {
				inputDelta = &parts[i]
			}
			if parts[i].Type == provider.PartToolCall {
				toolCall = &parts[i]
			}
		}
		require.NotNil(t, inputDelta)
		assert.Contains(t, inputDelta.Delta, `"type": "programmatic-tool-call"`)
		assert.Contains(t, inputDelta.Delta, `"code": "print('hi')"`)
		require.NotNil(t, toolCall)
		assert.JSONEq(t, `{"type":"programmatic-tool-call","code":"print('hi')"}`, toolCall.Input)
	})

	t.Run("existing_type_not_overwritten", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"code_execution"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"type\": \"bash\", \"code\": \"ls\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var toolCall *provider.StreamPart
		for i := range parts {
			if parts[i].Type == provider.PartToolCall {
				toolCall = &parts[i]
			}
		}
		require.NotNil(t, toolCall)
		assert.Contains(t, toolCall.Input, `"type": "bash"`)
		assert.NotContains(t, toolCall.Input, "programmatic-tool-call")
	})

	t.Run("partial_code_delta_injects_programmatic", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"code_execution"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"code\": \"print("}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"1)\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var inputDelta, toolCall *provider.StreamPart
		for i := range parts {
			if parts[i].Type == provider.PartToolInputDelta && inputDelta == nil {
				inputDelta = &parts[i]
			}
			if parts[i].Type == provider.PartToolCall {
				toolCall = &parts[i]
			}
		}
		require.NotNil(t, inputDelta)
		assert.Equal(t, `{"type": "programmatic-tool-call","code": "print(`, inputDelta.Delta)
		require.NotNil(t, toolCall)
		assert.JSONEq(t, `{"type":"programmatic-tool-call","code":"print(1)"}`, toolCall.Input)
	})

	t.Run("malformed_json_passes_through", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"code_execution"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{malformed"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var toolCall *provider.StreamPart
		for i := range parts {
			if parts[i].Type == provider.PartToolCall {
				toolCall = &parts[i]
			}
		}
		require.NotNil(t, toolCall)
		assert.Equal(t, "{malformed", toolCall.Input)
		assert.NotContains(t, toolCall.Input, "programmatic-tool-call")
	})
}

func TestStreamAdapter_CodeExecutionResultBlocks(t *testing.T) {
	t.Run("code_execution_tool_result", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"code_execution_tool_result","tool_use_id":"stu_1","content":{"type":"code_execution_result","stdout":"hello\n","stderr":"","return_code":0}}}`),
		}

		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.code_execution_20250825",
			Name: "code_exec",
		}})
		parts := collectPartsWithMapping(events, mapping)

		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartToolResult, parts[0].Type)
		assert.Equal(t, "stu_1", parts[0].ToolCallID)
		assert.Equal(t, "code_exec", parts[0].ToolName)
		assert.True(t, parts[0].ProviderExecuted)
	})

	t.Run("bash_code_execution_tool_result", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"bash_code_execution_tool_result","tool_use_id":"stu_2","content":{"type":"bash_code_execution_result","stdout":"done","stderr":"","return_code":0}}}`),
		}

		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.code_execution_20250825",
			Name: "code_exec",
		}})
		parts := collectPartsWithMapping(events, mapping)

		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartToolResult, parts[0].Type)
		assert.Equal(t, "stu_2", parts[0].ToolCallID)
		assert.Equal(t, "code_exec", parts[0].ToolName)
	})

	t.Run("text_editor_code_execution_tool_result", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text_editor_code_execution_tool_result","tool_use_id":"stu_3","content":{"type":"text_editor_code_execution_view_result","text":"file contents"}}}`),
		}

		mapping := newToolNameMapping([]provider.Tool{provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.code_execution_20250825",
			Name: "code_exec",
		}})
		parts := collectPartsWithMapping(events, mapping)

		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartToolResult, parts[0].Type)
		assert.Equal(t, "stu_3", parts[0].ToolCallID)
		assert.Equal(t, "code_exec", parts[0].ToolName)
	})

	t.Run("code_execution_tool_result_error_sets_isError", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"code_execution_tool_result","tool_use_id":"stu_1","content":{"type":"code_execution_tool_result_error","error_code":"execution_time_exceeded"}}}`),
		}
		parts := collectPartsWithMapping(events, toolNameMapping{})

		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartToolResult, parts[0].Type)
		assert.True(t, parts[0].IsError)
		assert.Contains(t, string(parts[0].Result), `"errorCode":"execution_time_exceeded"`)
	})

	t.Run("code_execution_tool_result_normalizes_fields", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"code_execution_tool_result","tool_use_id":"stu_1","content":{"type":"code_execution_result","stdout":"out","stderr":"err","return_code":1}}}`),
		}
		parts := collectPartsWithMapping(events, toolNameMapping{})

		require.Len(t, parts, 1)
		assert.False(t, parts[0].IsError)
		var parsed map[string]any
		require.NoError(t, json.Unmarshal(parts[0].Result, &parsed))
		assert.Equal(t, "code_execution_result", parsed["type"])
		assert.Equal(t, "out", parsed["stdout"])
		assert.Equal(t, "err", parsed["stderr"])
		assert.Equal(t, float64(1), parsed["return_code"])
		assert.NotNil(t, parsed["content"], "null content should be normalized to empty array")
	})
}

func TestStreamAdapter_PrePopulatedInput(t *testing.T) {
	t.Run("tool_use_with_pre_populated_input", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"my_tool","input":{"key":"value"}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}

		parts := collectParts(events)

		var toolCall *provider.StreamPart
		for i := range parts {
			if parts[i].Type == provider.PartToolCall {
				toolCall = &parts[i]
			}
		}
		require.NotNil(t, toolCall)
		assert.Contains(t, toolCall.Input, `"key":"value"`)
	})

	t.Run("message_start_with_tool_use_content", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"tool_use","id":"toolu_1","name":"my_tool","input":{"key":"val"}}],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`),
		}

		parts := collectParts(events)

		var toolInputStart, toolInputDelta, toolInputEnd, toolCall *provider.StreamPart
		for i := range parts {
			switch parts[i].Type {
			case provider.PartToolInputStart:
				toolInputStart = &parts[i]
			case provider.PartToolInputDelta:
				toolInputDelta = &parts[i]
			case provider.PartToolInputEnd:
				toolInputEnd = &parts[i]
			case provider.PartToolCall:
				toolCall = &parts[i]
			}
		}
		require.NotNil(t, toolInputStart, "expected PartToolInputStart")
		assert.Equal(t, "toolu_1", toolInputStart.ID)
		assert.Equal(t, "my_tool", toolInputStart.ToolName)

		require.NotNil(t, toolInputDelta, "expected PartToolInputDelta")
		assert.Contains(t, toolInputDelta.Delta, `"key":"val"`)

		require.NotNil(t, toolInputEnd, "expected PartToolInputEnd")
		require.NotNil(t, toolCall, "expected PartToolCall")
		assert.Equal(t, "toolu_1", toolCall.ToolCallID)
		assert.Equal(t, "my_tool", toolCall.ToolName)
	})

	t.Run("message_start_with_caller_metadata", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"tool_use","id":"toolu_1","name":"my_tool","input":{},"caller":{"type":"code_execution_20250825","tool_id":"toolu_456"}}],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`),
		}

		parts := collectParts(events)

		var toolCall *provider.StreamPart
		for i := range parts {
			if parts[i].Type == provider.PartToolCall {
				toolCall = &parts[i]
			}
		}
		require.NotNil(t, toolCall)
		require.NotNil(t, toolCall.ProviderMetadata)
		var meta map[string]any
		require.NoError(t, json.Unmarshal(toolCall.ProviderMetadata["anthropic"], &meta))
		caller := meta["caller"].(map[string]any)
		assert.Equal(t, "code_execution_20250825", caller["type"])
		assert.Equal(t, "toolu_456", caller["toolId"])
	})
}

// TestStreamAdapter_MarkCodeExecutionDynamic covers the upstream
// hasWebTool20260209WithoutCodeExecution behavior on the streaming path: when
// a 20260209 web tool is configured without an explicit code_execution tool,
// implicit code_execution server_tool_use blocks must carry dynamic: true on
// both tool-input-start and the final tool-call events
// (anthropic-language-model.ts:1705-1714 and :2104-2121).
func TestStreamAdapter_MarkCodeExecutionDynamic(t *testing.T) {
	codeExecutionEvents := []anthropic.BetaRawMessageStreamEventUnion{
		unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"code_execution"}}`),
		unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"code\":\"print('hi')\"}"}}`),
		unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
	}

	collect := func(t *testing.T, markDynamic bool, events []anthropic.BetaRawMessageStreamEventUnion) []provider.StreamPart {
		t.Helper()
		adapter := &streamAdapter{
			blocks:                   make(map[int64]*blockState),
			mapping:                  toolNameMapping{},
			serverToolCalls:          make(map[string]string),
			mcpToolCalls:             make(map[string]mcpToolCallInfo),
			markCodeExecutionDynamic: markDynamic,
			generateID:               defaultGenerateID,
		}
		ch := make(chan provider.StreamPart, 32)
		for _, e := range events {
			require.NoError(t, adapter.handleEvent(e, ch))
		}
		close(ch)
		var out []provider.StreamPart
		for p := range ch {
			out = append(out, p)
		}
		return out
	}

	findFirst := func(parts []provider.StreamPart, kind provider.StreamPartType) *provider.StreamPart {
		for i := range parts {
			if parts[i].Type == kind {
				return &parts[i]
			}
		}
		return nil
	}

	t.Run("code_execution gets dynamic when mark=true", func(t *testing.T) {
		parts := collect(t, true, codeExecutionEvents)

		start := findFirst(parts, provider.PartToolInputStart)
		require.NotNil(t, start)
		require.NotNil(t, start.Dynamic, "tool-input-start must carry dynamic=true")
		assert.True(t, *start.Dynamic)
		assert.True(t, start.ProviderExecuted)

		call := findFirst(parts, provider.PartToolCall)
		require.NotNil(t, call)
		require.NotNil(t, call.Dynamic, "tool-call must carry dynamic=true")
		assert.True(t, *call.Dynamic)
		assert.True(t, call.ProviderExecuted)
	})

	t.Run("code_execution stays plain when mark=false", func(t *testing.T) {
		parts := collect(t, false, codeExecutionEvents)

		start := findFirst(parts, provider.PartToolInputStart)
		require.NotNil(t, start)
		assert.Nil(t, start.Dynamic)

		call := findFirst(parts, provider.PartToolCall)
		require.NotNil(t, call)
		assert.Nil(t, call.Dynamic)
	})

	t.Run("bash_code_execution maps to code_execution and gets dynamic", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"bash_code_execution"}}`),
			unmarshalEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"code\":\"ls\"}"}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}
		parts := collect(t, true, events)

		call := findFirst(parts, provider.PartToolCall)
		require.NotNil(t, call)
		require.NotNil(t, call.Dynamic)
		assert.True(t, *call.Dynamic)
	})

	t.Run("non-code_execution server tool is not marked dynamic", func(t *testing.T) {
		events := []anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"stu_1","name":"web_fetch","input":{"url":"https://example.com"}}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
		}
		parts := collect(t, true, events)

		start := findFirst(parts, provider.PartToolInputStart)
		require.NotNil(t, start)
		assert.Nil(t, start.Dynamic, "web_fetch must not be marked dynamic")

		call := findFirst(parts, provider.PartToolCall)
		require.NotNil(t, call)
		assert.Nil(t, call.Dynamic)
	})
}
