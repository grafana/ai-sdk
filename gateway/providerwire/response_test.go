package providerwire

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeGenerateResult_FullRoundTrip(t *testing.T) {
	ts := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	full := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "hello"},
			{Type: provider.ContentReasoning, Text: "thinking"},
			{Type: provider.ContentToolCall, ID: "tc_1", ToolName: "search", Input: json.RawMessage(`{"q":"x"}`)},
			{Type: provider.ContentToolResult, ToolCallID: "tc_1", ToolName: "search", Result: json.RawMessage(`{"ok":true}`), ProviderExecuted: true},
			{Type: provider.ContentSource, ID: "src_1", SourceType: provider.SourceTypeURL, URL: "https://example.com"},
			{Type: provider.ContentFile, MediaType: "image/png", Data: &provider.DataContent{URL: "https://example.com/f"}},
			{Type: provider.ContentReasoningFile, MediaType: "image/png", Data: &provider.DataContent{Base64: "AAEC"}},
			{Type: provider.ContentCustom, Kind: "anthropic.cache-control"},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: ptrInt(100), CacheRead: ptrInt(50)},
			OutputTokens: provider.OutputTokenUsage{Total: ptrInt(20)},
		},
		ProviderMetadata: provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"model":"claude"}`),
		},
		Warnings: []provider.Warning{
			{Type: provider.WarnUnsupported, Feature: "logprobs"},
		},
		Request: &provider.RequestMetadata{Body: json.RawMessage(`{"model":"x"}`)},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{
				ID:        "msg_1",
				ModelID:   "claude-x",
				Timestamp: ts,
			},
			Headers: map[string]string{"x-request-id": "abc"},
			Body:    json.RawMessage(`{"id":"msg_1"}`),
		},
	}

	data, err := EncodeGenerateResult(full)
	require.NoError(t, err)

	got, err := DecodeGenerateResult(data)
	require.NoError(t, err)
	assert.Equal(t, full, got)
}

func TestEncodeGenerateResult_NilReturnsError(t *testing.T) {
	_, err := EncodeGenerateResult(nil)
	assert.Error(t, err)
}

func TestEncodeGenerateResult_OmitsEmptyDataOnNonFilePart(t *testing.T) {
	data, err := EncodeGenerateResult(&provider.GenerateResult{
		Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello"}},
	})
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"data":{}`)
}
