package agentobservability

import (
	"encoding/json"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentToAgento11yOutput_TextAndToolCall(t *testing.T) {
	content := []provider.GenerateContentPart{
		{Type: provider.ContentText, Text: "Here is the weather:"},
		{
			Type:       provider.ContentToolCall,
			ToolCallID: "tc-1",
			ToolName:   "lookup",
			Input:      json.RawMessage(`{"q":"sf"}`),
		},
	}
	msgs := contentToAgento11yOutput(content)
	require.Len(t, msgs, 1, "single assistant message expected")
	assert.Equal(t, agento11y.RoleAssistant, msgs[0].Role)
	require.Len(t, msgs[0].Parts, 2)
	assert.Equal(t, agento11y.PartKindText, msgs[0].Parts[0].Kind)
	assert.Equal(t, "Here is the weather:", msgs[0].Parts[0].Text)
	assert.Equal(t, agento11y.PartKindToolCall, msgs[0].Parts[1].Kind)
	require.NotNil(t, msgs[0].Parts[1].ToolCall)
	assert.Equal(t, "tc-1", msgs[0].Parts[1].ToolCall.ID)
}

func TestContentToAgento11yOutput_Reasoning(t *testing.T) {
	content := []provider.GenerateContentPart{
		{Type: provider.ContentReasoning, Text: "let me think…"},
		{Type: provider.ContentText, Text: "the answer is 42"},
	}
	msgs := contentToAgento11yOutput(content)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Parts, 2)
	assert.Equal(t, agento11y.PartKindThinking, msgs[0].Parts[0].Kind)
	assert.Equal(t, "let me think…", msgs[0].Parts[0].Thinking)
	assert.Equal(t, "thinking", msgs[0].Parts[0].Metadata.ProviderType)
}

func TestContentToAgento11yOutput_SkipsEmptyReasoning(t *testing.T) {
	msgs := contentToAgento11yOutput([]provider.GenerateContentPart{
		{Type: provider.ContentReasoning},
		{Type: provider.ContentText, Text: "answer"},
	})

	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Parts, 1)
	assert.Equal(t, agento11y.PartKindText, msgs[0].Parts[0].Kind)
}

func TestContentToAgento11yOutput_ToolResultSplit(t *testing.T) {
	content := []provider.GenerateContentPart{
		{Type: provider.ContentText, Text: "Looking that up…"},
		{
			Type:       provider.ContentToolResult,
			ToolCallID: "tc-1",
			ToolName:   "lookup",
			Result:     json.RawMessage(`{"output":"ok"}`),
		},
	}
	msgs := contentToAgento11yOutput(content)
	require.Len(t, msgs, 2, "assistant + tool message split")
	assert.Equal(t, agento11y.RoleAssistant, msgs[0].Role)
	assert.Equal(t, agento11y.RoleTool, msgs[1].Role)
}

func TestContentToAgento11yOutput_CodeExecutionResultSubtype(t *testing.T) {
	tools := []provider.Tool{{
		Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250825", Name: "run_code",
	}}
	messages := contentToAgento11yOutputWithTools([]provider.GenerateContentPart{{
		Type: provider.ContentToolResult, ToolCallID: "call-1", ToolName: "run_code",
		ProviderExecuted: true,
		Result:           json.RawMessage(`{"type":"bash_code_execution_result"}`),
	}}, tools)
	require.Len(t, messages, 1)
	assert.Equal(t, "bash_code_execution_tool_result", messages[0].Parts[0].Metadata.ProviderType)
}

func TestContentToAgento11yOutput_FileParts(t *testing.T) {
	pngData := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	content := []provider.GenerateContentPart{
		{
			Type:      provider.ContentFile,
			MediaType: "application/octet-stream",
			Filename:  "plot.png",
			Data:      &provider.DataContent{Bytes: pngData},
		},
		{
			Type:      provider.ContentReasoningFile,
			MediaType: "video/*",
			Filename:  "trace.webm",
			Data:      &provider.DataContent{Base64: "AQID"},
		},
	}

	msgs := contentToAgento11yOutput(content)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Parts, 2)

	file := msgs[0].Parts[0]
	assert.Equal(t, agento11y.PartKindMedia, file.Kind)
	require.NotNil(t, file.Media)
	assert.Equal(t, "image", file.Media.Kind)
	assert.Equal(t, "image/png", file.Media.MIMEType)
	assert.Equal(t, "plot.png", file.Media.Name)
	assert.Equal(t, "file", file.Metadata.ProviderType)

	reasoningFile := msgs[0].Parts[1]
	assert.Equal(t, agento11y.PartKindMedia, reasoningFile.Kind)
	require.NotNil(t, reasoningFile.Media)
	assert.Equal(t, "video", reasoningFile.Media.Kind)
	assert.Equal(t, "data:video/webm;base64,AQID", reasoningFile.Media.URL)
	assert.Equal(t, "reasoning_file", reasoningFile.Metadata.ProviderType)
}

func TestContentToAgento11yOutput_SniffsInlineBytes(t *testing.T) {
	pngData := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	msgs := contentToAgento11yOutput([]provider.GenerateContentPart{{
		Type:      provider.ContentFile,
		MediaType: "application/octet-stream",
		Data:      &provider.DataContent{Bytes: pngData},
	}})

	require.Len(t, msgs, 1)
	media := msgs[0].Parts[0].Media
	require.NotNil(t, media)
	assert.Equal(t, "image/png", media.MIMEType)
	assert.Equal(t, "data:image/png;base64,iVBORw0KGgo=", media.URL)
}

func TestContentToAgento11yOutput_ValidDataURLParameters(t *testing.T) {
	const mediaURL = "data:image/svg+xml;charset=utf-8,%3Csvg%3E%3C/svg%3E"
	msgs := contentToAgento11yOutput([]provider.GenerateContentPart{{
		Type:      provider.ContentFile,
		MediaType: "image/svg+xml",
		Data:      &provider.DataContent{URL: mediaURL},
	}})

	require.Len(t, msgs, 1)
	media := msgs[0].Parts[0].Media
	require.NotNil(t, media)
	assert.Equal(t, mediaURL, media.URL)
	assert.Equal(t, "image/svg+xml", media.MIMEType)
}

func TestContentToAgento11yOutput_PercentEscapedBase64DataURL(t *testing.T) {
	const mediaURL = "data:image/png;base64,iVBORw0KGgo%3D"
	msgs := contentToAgento11yOutput([]provider.GenerateContentPart{{
		Type:      provider.ContentFile,
		MediaType: "image/png",
		Data:      &provider.DataContent{URL: mediaURL},
	}})

	require.Len(t, msgs, 1)
	media := msgs[0].Parts[0].Media
	require.NotNil(t, media)
	assert.Equal(t, mediaURL, media.URL)
	assert.Equal(t, "image/png", media.MIMEType)
}

func TestContentToAgento11yOutput_RemoteURLPathInference(t *testing.T) {
	const mediaURL = "https://cdn.example.com/generated/photo.png?X-Amz-Signature=secret"
	msgs := contentToAgento11yOutput([]provider.GenerateContentPart{{
		Type:      provider.ContentFile,
		MediaType: "image/*",
		Data:      &provider.DataContent{URL: mediaURL},
	}})

	require.Len(t, msgs, 1)
	media := msgs[0].Parts[0].Media
	require.NotNil(t, media)
	assert.Equal(t, mediaURL, media.URL)
	assert.Equal(t, "image/png", media.MIMEType)
}

func TestContentToAgento11yOutput_UnsupportedOrAmbiguousFile(t *testing.T) {
	tests := []struct {
		name string
		part provider.GenerateContentPart
	}{
		{
			name: "query filename is ignored",
			part: provider.GenerateContentPart{Type: provider.ContentFile, MediaType: "application/octet-stream", Data: &provider.DataContent{
				URL: "https://cdn.example.com/generated?file=photo.png",
			}},
		},
		{
			name: "generic media without evidence",
			part: provider.GenerateContentPart{Type: provider.ContentFile, MediaType: "image/*", Data: &provider.DataContent{
				Base64: "AQID",
			}},
		},
		{
			name: "declared and data URL MIME conflict",
			part: provider.GenerateContentPart{Type: provider.ContentFile, MediaType: "video/mp4", Data: &provider.DataContent{
				URL: "data:image/png;base64,AQID",
			}},
		},
		{
			name: "audio is unsupported",
			part: provider.GenerateContentPart{Type: provider.ContentFile, MediaType: "audio/mpeg", Data: &provider.DataContent{
				Base64: "AQID",
			}},
		},
		{
			name: "document is unsupported",
			part: provider.GenerateContentPart{Type: provider.ContentFile, MediaType: "application/pdf", Data: &provider.DataContent{
				Bytes: []byte("%PDF-1.7"),
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Nil(t, contentToAgento11yOutput([]provider.GenerateContentPart{tc.part}))
		})
	}
}

func TestContentToAgento11yOutput_Empty(t *testing.T) {
	assert.Nil(t, contentToAgento11yOutput(nil))
	assert.Nil(t, contentToAgento11yOutput([]provider.GenerateContentPart{
		{Type: provider.ContentSource},
		{Type: provider.ContentFile},
	}), "parts without convertible payloads skip and produce no message")
}

func TestUsageToAgento11y(t *testing.T) {
	in := 100
	out := 200
	cacheRead := 50
	cacheWrite := 25
	reasoning := 15
	usage := provider.Usage{
		InputTokens: provider.InputTokenUsage{
			Total:      &in,
			CacheRead:  &cacheRead,
			CacheWrite: &cacheWrite,
		},
		OutputTokens: provider.OutputTokenUsage{
			Total:     &out,
			Reasoning: &reasoning,
		},
	}
	got := usageToAgento11y(usage)
	assert.Equal(t, int64(100), got.InputTokens)
	assert.Equal(t, int64(200), got.OutputTokens)
	assert.Equal(t, int64(300), got.TotalTokens)
	assert.Equal(t, int64(50), got.CacheReadInputTokens)
	assert.Equal(t, int64(25), got.CacheWriteInputTokens)
	assert.Equal(t, int64(15), got.ReasoningTokens)
}

func TestUsageToAgento11y_Zero(t *testing.T) {
	got := usageToAgento11y(provider.Usage{})
	assert.Equal(t, agento11y.TokenUsage{}, got)
}

func TestMetadataFromUsage_ServerToolUse(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]any
	}{
		{
			name: "search and fetch",
			raw:  `{"server_tool_use":{"web_search_requests":2,"web_fetch_requests":1}}`,
			want: map[string]any{
				MetadataServerToolUseWebSearchRequests: int64(2),
				MetadataServerToolUseWebFetchRequests:  int64(1),
				MetadataServerToolUseTotalRequests:     int64(3),
			},
		},
		{
			name: "search only",
			raw:  `{"server_tool_use":{"web_search_requests":1}}`,
			want: map[string]any{
				MetadataServerToolUseWebSearchRequests: int64(1),
				MetadataServerToolUseTotalRequests:     int64(1),
			},
		},
		{name: "zero", raw: `{"server_tool_use":{"web_search_requests":0}}`},
		{name: "negative", raw: `{"server_tool_use":{"web_search_requests":-1}}`},
		{name: "malformed", raw: `{`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := metadataFromUsage(provider.Usage{Raw: json.RawMessage(tc.raw)})
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestProviderTypeForGenerateToolCall(t *testing.T) {
	tests := []struct {
		name string
		part provider.GenerateContentPart
		want string
	}{
		{name: "client tool", part: provider.GenerateContentPart{ToolName: "lookup"}, want: "tool_use"},
		{name: "server tool", part: provider.GenerateContentPart{ToolName: "web_search", ProviderExecuted: true}, want: "server_tool_use"},
		{name: "tool search", part: provider.GenerateContentPart{ToolName: "tool_search_tool_bm25", ProviderExecuted: true}, want: "tool_search_tool_bm25"},
		{
			name: "mcp",
			part: provider.GenerateContentPart{
				ToolName:         "remote_lookup",
				ProviderExecuted: true,
				ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use"}`)},
			},
			want: "mcp_tool_use",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, providerTypeForGenerateToolCall(tc.part, nil))
		})
	}
}

func TestContentToAgento11yOutput_ProviderToolAliases(t *testing.T) {
	tools := []provider.Tool{
		{Type: provider.ToolTypeProvider, ID: "anthropic.tool_search_bm25_20251119", Name: "find_tools"},
		{Type: provider.ToolTypeProvider, ID: "anthropic.web_search_20250305", Name: "search_docs"},
	}
	msgs := contentToAgento11yOutputWithTools([]provider.GenerateContentPart{
		{Type: provider.ContentToolCall, ToolCallID: "call-1", ToolName: "find_tools", ProviderExecuted: true},
		{Type: provider.ContentToolResult, ToolCallID: "call-1", ToolName: "find_tools"},
		{Type: provider.ContentToolResult, ToolCallID: "call-2", ToolName: "search_docs"},
	}, tools)

	require.Len(t, msgs, 2)
	assert.Equal(t, "tool_search_tool_bm25", msgs[0].Parts[0].Metadata.ProviderType)
	require.Len(t, msgs[1].Parts, 2)
	assert.Equal(t, "tool_search_tool_bm25_tool_result", msgs[1].Parts[0].Metadata.ProviderType)
	assert.Equal(t, "web_search_tool_result", msgs[1].Parts[1].Metadata.ProviderType)
}

func TestFinishReasonToAgento11yStop(t *testing.T) {
	tests := []struct {
		name string
		in   provider.FinishReason
		want string
	}{
		{"stop", provider.FinishReason{Unified: provider.FinishReasonStop}, stopReasonEndTurn},
		{"length", provider.FinishReason{Unified: provider.FinishReasonLength}, stopReasonMaxTokens},
		{"tool-calls", provider.FinishReason{Unified: provider.FinishReasonToolCalls}, stopReasonToolUse},
		{"content-filter", provider.FinishReason{Unified: provider.FinishReasonContentFilter}, stopReasonOther},
		{"error", provider.FinishReason{Unified: provider.FinishReasonError}, stopReasonError},
		{"other", provider.FinishReason{Unified: provider.FinishReasonOther}, stopReasonOther},
		{
			"raw preserved when present",
			provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "stop_sequence"},
			stopReasonStopSequence,
		},
		{
			"raw wins over unified",
			provider.FinishReason{Unified: provider.FinishReasonLength, Raw: "pause_turn"},
			"pause_turn",
		},
		{"empty", provider.FinishReason{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := finishReasonToAgento11yStop(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}
