package aisdk

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkTypeConstants(t *testing.T) {
	types := []ChunkType{
		ChunkStart, ChunkFinish, ChunkAbort, ChunkStartStep, ChunkFinishStep, ChunkMessageMetadata,
		ChunkTextStart, ChunkTextDelta, ChunkTextEnd,
		ChunkReasoningStart, ChunkReasoningDelta, ChunkReasoningEnd, ChunkReasoningFile,
		ChunkToolInputStart, ChunkToolInputDelta, ChunkToolInputAvailable, ChunkToolInputError,
		ChunkToolApprovalRequest, ChunkToolApprovalResponse,
		ChunkToolOutputDenied,
		ChunkToolOutputAvailable, ChunkToolOutputError,
		ChunkSourceURL, ChunkSourceDocument,
		ChunkFile, ChunkError,
	}
	unique := make(map[ChunkType]bool)
	for _, ct := range types {
		assert.False(t, unique[ct], "duplicate chunk type: %q", ct)
		unique[ct] = true
	}
}

func TestChunkJSON(t *testing.T) {
	t.Run("text delta", func(t *testing.T) {
		c := TextDeltaChunk("b1", "hello")
		b, err := json.Marshal(c)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		assert.Equal(t, "text-delta", m["type"])
		assert.Equal(t, "hello", m["delta"])
	})

	t.Run("data chunk", func(t *testing.T) {
		c := DataChunk("status", json.RawMessage(`{"step":"done"}`), true)
		b, err := json.Marshal(c)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		assert.Equal(t, "data-status", m["type"])
		assert.Equal(t, true, m["transient"])
	})

	t.Run("tool input available", func(t *testing.T) {
		c := ToolInputAvailableChunk("call-1", "weather", json.RawMessage(`{"city":"NYC"}`))
		b, err := json.Marshal(c)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		assert.Equal(t, "tool-input-available", m["type"])
		assert.Equal(t, "weather", m["toolName"])
	})

	t.Run("tool approval response false", func(t *testing.T) {
		c := UIMessageChunk{Type: ChunkToolApprovalResponse, ApprovalID: "apr_1", Approved: false, Reason: "unsafe"}
		b, err := json.Marshal(c)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		assert.Equal(t, "tool-approval-response", m["type"])
		assert.Equal(t, false, m["approved"])
		assert.Equal(t, "unsafe", m["reason"])
	})

	t.Run("tool approval request signature", func(t *testing.T) {
		c := UIMessageChunk{Type: ChunkToolApprovalRequest, ApprovalID: "apr_1", ToolCallID: "call_1", Signature: "sig_1"}
		b, err := json.Marshal(c)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		assert.Equal(t, "tool-approval-request", m["type"])
		assert.Equal(t, "sig_1", m["signature"])
	})

	t.Run("tool metadata", func(t *testing.T) {
		c := UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "call_1", ToolName: "search", Input: json.RawMessage(`{}`), ToolMetadata: json.RawMessage(`{"display":"Search"}`)}
		b, err := json.Marshal(c)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		assert.Equal(t, "tool-input-available", m["type"])
		raw, err := json.Marshal(m["toolMetadata"])
		require.NoError(t, err)
		assert.JSONEq(t, `{"display":"Search"}`, string(raw))
	})
}

func TestHelperConstructors(t *testing.T) {
	cases := []struct {
		chunk    UIMessageChunk
		wantType ChunkType
	}{
		{TextStartChunk("b1"), ChunkTextStart},
		{TextEndChunk("b1"), ChunkTextEnd},
		{ReasoningStartChunk("r1"), ChunkReasoningStart},
		{ReasoningDeltaChunk("r1", "d"), ChunkReasoningDelta},
		{ReasoningEndChunk("r1"), ChunkReasoningEnd},
		{ToolInputStartChunk("c1", "w"), ChunkToolInputStart},
		{ToolInputDeltaChunk("c1", "d"), ChunkToolInputDelta},
		{UIMessageChunk{Type: ChunkToolApprovalRequest, ApprovalID: "apr", ToolCallID: "c1"}, ChunkToolApprovalRequest},
		{UIMessageChunk{Type: ChunkToolOutputDenied, ToolCallID: "c1"}, ChunkToolOutputDenied},
		{ToolOutputAvailableChunk("c1", json.RawMessage(`{}`)), ChunkToolOutputAvailable},
		{ToolOutputErrorChunk("c1", "fail"), ChunkToolOutputError},
		{SourceURLChunk("s1", "https://example.com"), ChunkSourceURL},
		{SourceDocumentChunk("s1", "application/pdf", "doc"), ChunkSourceDocument},
		{FileChunk("https://f.com/img.png", "image/png"), ChunkFile},
		{ReasoningFileChunk("https://f.com/reasoning.png", "image/png"), ChunkReasoningFile},
		{ErrorChunk("oops"), ChunkError},
		{StartChunk("msg-1"), ChunkStart},
		{FinishChunk("stop"), ChunkFinish},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantType, tc.chunk.Type)
	}
}

func TestUIMessageChunkUnmarshal(t *testing.T) {
	t.Run("accepts unknown fields on known chunk", func(t *testing.T) {
		var chunk UIMessageChunk
		require.NoError(t, json.Unmarshal([]byte(`{"type":"text-delta","id":"t1","delta":"hi","futureField":true}`), &chunk))
		assert.Equal(t, ChunkTextDelta, chunk.Type)
		assert.Equal(t, "hi", chunk.Delta)
	})

	t.Run("rejects unknown chunk type", func(t *testing.T) {
		var chunk UIMessageChunk
		err := json.Unmarshal([]byte(`{"type":"future-chunk","value":true}`), &chunk)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported UI message chunk type "future-chunk"`)
	})

	t.Run("decodes data chunk discriminator", func(t *testing.T) {
		var chunk UIMessageChunk
		require.NoError(t, json.Unmarshal([]byte(`{"type":"data-weather","data":{"temperature":72}}`), &chunk))
		assert.Equal(t, ChunkData, chunk.Type)
		assert.Equal(t, "weather", chunk.DataName)
		assert.JSONEq(t, `{"temperature":72}`, string(chunk.Data))
	})
}

func TestDataChunkConstructorSetsType(t *testing.T) {
	c := DataChunk("status", json.RawMessage(`{"ok":true}`), false)
	assert.Equal(t, ChunkData, c.Type)
	assert.Equal(t, "status", c.DataName)
}

func TestTranslateToChunks_ProviderMetadataPassthrough(t *testing.T) {
	meta := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"caller":{"type":"direct"}}`),
	}

	t.Run("StreamToolCall_to_ChunkToolInputAvailable", func(t *testing.T) {
		part := StreamToolCall{
			ToolCallID:       "call_1",
			ToolName:         "search",
			Input:            json.RawMessage(`{}`),
			ProviderMetadata: meta,
		}
		chunks := translateToChunks(part, uiMessageStreamConfig{})
		require.Len(t, chunks, 1)
		assert.Equal(t, ChunkToolInputAvailable, chunks[0].Type)
		assert.Equal(t, meta, chunks[0].ProviderMetadata)
	})

	t.Run("StreamToolResult_to_ChunkToolOutputAvailable", func(t *testing.T) {
		part := StreamToolResult{
			ToolCallID:       "call_1",
			ToolName:         "search",
			Output:           json.RawMessage(`{"result":"ok"}`),
			ProviderMetadata: meta,
		}
		chunks := translateToChunks(part, uiMessageStreamConfig{})
		require.Len(t, chunks, 1)
		assert.Equal(t, ChunkToolOutputAvailable, chunks[0].Type)
		assert.Equal(t, meta, chunks[0].ProviderMetadata)
	})

	t.Run("StreamCustom_to_ChunkCustom", func(t *testing.T) {
		meta := provider.ProviderMetadata{"openai": json.RawMessage(`{"type":"compaction"}`)}
		chunks := translateToChunks(StreamCustom{Kind: "openai.compaction", ProviderMetadata: meta}, uiMessageStreamConfig{})
		require.Len(t, chunks, 1)
		assert.Equal(t, ChunkCustom, chunks[0].Type)
		assert.Equal(t, "openai.compaction", chunks[0].Kind)
		assert.Equal(t, meta, chunks[0].ProviderMetadata)
	})

	t.Run("StreamToolApprovalRequest_to_ChunkToolApprovalRequest", func(t *testing.T) {
		part := StreamToolApprovalRequest{ApprovalID: "apr_1", ToolCallID: "call_1"}
		chunks := translateToChunks(part, uiMessageStreamConfig{})
		require.Len(t, chunks, 1)
		assert.Equal(t, ChunkToolApprovalRequest, chunks[0].Type)
		assert.Equal(t, "apr_1", chunks[0].ApprovalID)
		assert.Equal(t, "call_1", chunks[0].ToolCallID)
	})

	t.Run("StreamToolApprovalResponse_to_ChunkToolApprovalResponse", func(t *testing.T) {
		part := StreamToolApprovalResponse{ApprovalID: "apr_1", Approved: false, Reason: "unsafe"}
		chunks := translateToChunks(part, uiMessageStreamConfig{})
		require.Len(t, chunks, 1)
		assert.Equal(t, ChunkToolApprovalResponse, chunks[0].Type)
		assert.False(t, chunks[0].Approved)
		assert.Equal(t, "unsafe", chunks[0].Reason)
	})

	t.Run("StreamToolOutputDenied_to_ChunkToolOutputDenied", func(t *testing.T) {
		part := StreamToolOutputDenied{ToolCallID: "call_1"}
		chunks := translateToChunks(part, uiMessageStreamConfig{})
		require.Len(t, chunks, 1)
		assert.Equal(t, ChunkToolOutputDenied, chunks[0].Type)
		assert.Equal(t, "call_1", chunks[0].ToolCallID)
	})

	t.Run("StreamToolCall_nil_metadata", func(t *testing.T) {
		part := StreamToolCall{
			ToolCallID: "call_2",
			ToolName:   "weather",
			Input:      json.RawMessage(`{}`),
		}
		chunks := translateToChunks(part, uiMessageStreamConfig{})
		require.Len(t, chunks, 1)
		assert.Nil(t, chunks[0].ProviderMetadata)
	})

	t.Run("StreamToolResult_nil_metadata", func(t *testing.T) {
		part := StreamToolResult{
			ToolCallID: "call_2",
			ToolName:   "weather",
			Output:     json.RawMessage(`{}`),
		}
		chunks := translateToChunks(part, uiMessageStreamConfig{})
		require.Len(t, chunks, 1)
		assert.Nil(t, chunks[0].ProviderMetadata)
	})
}

// Compile-time interface satisfaction checks for all TextStreamPart concrete types.
var (
	_ ReasoningOutput = ReasoningTextOutput{}
	_ ReasoningOutput = ReasoningFileOutput{}
	_ ContentPart     = ReasoningContent{}
	_ ContentPart     = FileContent{}
	_ ContentPart     = ReasoningFileContent{}

	_ TextStreamPart = StreamStart{}
	_ TextStreamPart = StreamStartStep{}
	_ TextStreamPart = StreamFinishStep{}
	_ TextStreamPart = StreamFinish{}
	_ TextStreamPart = StreamAbort{}
	_ TextStreamPart = StreamError{}
	_ TextStreamPart = StreamRaw{}
	_ TextStreamPart = StreamCustom{}
	_ TextStreamPart = StreamTextStart{}
	_ TextStreamPart = StreamTextDelta{}
	_ TextStreamPart = StreamTextEnd{}
	_ TextStreamPart = StreamReasoningStart{}
	_ TextStreamPart = StreamReasoningDelta{}
	_ TextStreamPart = StreamReasoningEnd{}
	_ TextStreamPart = StreamToolInputStart{}
	_ TextStreamPart = StreamToolInputDelta{}
	_ TextStreamPart = StreamToolInputEnd{}
	_ TextStreamPart = StreamSource{}
	_ TextStreamPart = StreamFile{}
	_ TextStreamPart = StreamReasoningFile{}
	_ TextStreamPart = StreamToolCall{}
	_ TextStreamPart = StreamToolApprovalRequest{}
	_ TextStreamPart = StreamToolApprovalResponse{}
	_ TextStreamPart = StreamToolOutputDenied{}
	_ TextStreamPart = StreamToolResult{}
	_ TextStreamPart = StreamToolError{}
)
