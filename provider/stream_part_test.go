package provider

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamPartTypeConstants(t *testing.T) {
	types := []StreamPartType{
		PartTextStart, PartTextDelta, PartTextEnd,
		PartReasoningStart, PartReasoningDelta, PartReasoningEnd,
		PartToolInputStart, PartToolInputDelta, PartToolInputEnd,
		PartToolCall, PartToolResult,
		PartSource, PartFile,
		PartStreamStart, PartResponseMeta, PartFinish,
		PartRaw, PartError,
		PartToolApprovalRequest,
		PartCustom, PartReasoningFile,
	}
	assert.Len(t, types, 21)

	unique := make(map[StreamPartType]bool)
	for _, typ := range types {
		assert.False(t, unique[typ], "duplicate stream part type: %q", typ)
		unique[typ] = true
	}
}

func TestStreamPartConstruction(t *testing.T) {
	p := StreamPart{
		Type:  PartTextDelta,
		ID:    "block-1",
		Delta: "hello",
	}
	assert.Equal(t, PartTextDelta, p.Type)
	assert.Equal(t, "block-1", p.ID)
	assert.Equal(t, "hello", p.Delta)
}

func TestStreamPart_Preliminary(t *testing.T) {
	t.Run("preliminary tool result", func(t *testing.T) {
		preliminary := true
		p := StreamPart{
			Type:        PartToolResult,
			ToolCallID:  "call_1",
			ToolName:    "preview",
			Preliminary: &preliminary,
		}
		assert.Equal(t, PartToolResult, p.Type)
		assert.NotNil(t, p.Preliminary)
		assert.True(t, *p.Preliminary)
	})

	t.Run("final tool result has nil preliminary", func(t *testing.T) {
		p := StreamPart{
			Type:       PartToolResult,
			ToolCallID: "call_1",
			ToolName:   "search",
		}
		assert.Nil(t, p.Preliminary)
	})
}

func TestStreamPart_PartError_CarriesAPICallError(t *testing.T) {
	apiErr := NewAPICallError(APICallErrorOptions{
		Message:    "rate limit",
		StatusCode: 429,
	})
	p := StreamPart{
		Type:         PartError,
		APICallError: apiErr,
	}
	data, err := json.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"type":"error"`)
	assert.Contains(t, string(data), `"isRetryable":true`)

	var decoded StreamPart
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.APICallError)
	assert.Equal(t, 429, decoded.APICallError.StatusCode)
	assert.True(t, decoded.APICallError.IsRetryable)
	assert.Equal(t, "rate limit", decoded.APICallError.Message)
}

func TestStreamPart_BytesFileData_RoundTrip(t *testing.T) {
	p := StreamPart{
		Type:      PartReasoningFile,
		Data:      &StreamFileData{Type: StreamFileDataTypeData, Bytes: []byte{0x01, 0x02, 0x03}},
		MediaType: "image/png",
	}
	data, err := json.Marshal(p)
	require.NoError(t, err)
	// File data is emitted as the upstream tagged union with the bytes
	// base64-encoded; the legacy flat "fileData" field is no longer emitted.
	assert.JSONEq(t, `{"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":"AQID"}}`, string(data))

	var decoded StreamPart
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.Data)
	assert.Equal(t, StreamFileData{Type: StreamFileDataTypeData, Base64: "AQID"}, *decoded.Data)
	assert.Equal(t, p.MediaType, decoded.MediaType)
	assert.Equal(t, p.Type, decoded.Type)
}

func TestStreamPart_ResponseMetadataWireShape(t *testing.T) {
	ts := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	p := StreamPart{
		Type:       PartResponseMeta,
		ResponseID: "resp_1",
		ModelID:    "model_1",
		Timestamp:  ts,
	}
	data, err := json.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"id":"resp_1"`)
	assert.NotContains(t, string(data), "responseId")

	var decoded StreamPart
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, p, decoded)
}

func TestSourceInfo_DocumentVariant(t *testing.T) {
	t.Run("document source has required fields", func(t *testing.T) {
		src := SourceInfo{
			SourceType: SourceTypeDocument,
			ID:         "doc_1",
			MediaType:  "application/pdf",
			Title:      "Research Paper",
			Filename:   "paper.pdf",
		}
		assert.Equal(t, SourceTypeDocument, src.SourceType)
		assert.Equal(t, "doc_1", src.ID)
		assert.Equal(t, "application/pdf", src.MediaType)
		assert.Equal(t, "Research Paper", src.Title)
		assert.Equal(t, "paper.pdf", src.Filename)
	})

	t.Run("url source unchanged", func(t *testing.T) {
		src := SourceInfo{
			SourceType: SourceTypeURL,
			ID:         "src_1",
			URL:        "https://example.com",
			Title:      "Example",
		}
		assert.Equal(t, SourceTypeURL, src.SourceType)
		assert.Equal(t, "https://example.com", src.URL)
	})

	t.Run("document source in stream part", func(t *testing.T) {
		p := StreamPart{
			Type: PartSource,
			Source: &SourceInfo{
				SourceType: SourceTypeDocument,
				ID:         "doc_2",
				MediaType:  "text/html",
				Title:      "Page Title",
			},
		}
		assert.Equal(t, PartSource, p.Type)
		assert.Equal(t, SourceTypeDocument, p.Source.SourceType)
		assert.Equal(t, "Page Title", p.Source.Title)
	})
}
