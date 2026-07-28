package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushCount int
}

func (r *flushRecorder) Flush() {
	r.flushCount++
	r.ResponseRecorder.Flush()
}

func typeName(p TextStreamPart) string {
	switch p.(type) {
	case StreamStart:
		return "start"
	case StreamStartStep:
		return "start-step"
	case StreamTextStart:
		return "text-start"
	case StreamTextDelta:
		return "text-delta"
	case StreamTextEnd:
		return "text-end"
	case StreamReasoningStart:
		return "reasoning-start"
	case StreamReasoningDelta:
		return "reasoning-delta"
	case StreamReasoningEnd:
		return "reasoning-end"
	case StreamToolInputStart:
		return "tool-input-start"
	case StreamToolInputDelta:
		return "tool-input-delta"
	case StreamToolInputEnd:
		return "tool-input-end"
	case StreamToolCall:
		return "tool-call"
	case StreamToolApprovalRequest:
		return "tool-approval-request"
	case StreamToolApprovalResponse:
		return "tool-approval-response"
	case StreamToolOutputDenied:
		return "tool-output-denied"
	case StreamToolResult:
		return "tool-result"
	case StreamToolError:
		return "tool-error"
	case StreamFinishStep:
		return "finish-step"
	case StreamFinish:
		return "finish"
	case StreamAbort:
		return "abort"
	case StreamError:
		return "error"
	case StreamRaw:
		return "raw"
	case StreamSource:
		return "source"
	case StreamFile:
		return "file"
	case StreamReasoningFile:
		return "reasoning-file"
	default:
		return fmt.Sprintf("unknown(%T)", p)
	}
}

func TestPipeUIMessageStreamToResponse(t *testing.T) {
	ch := make(chan UIMessageChunk, 5)
	ch <- UIMessageChunk{Type: ChunkStart}
	ch <- TextDeltaChunk("t1", "hello")
	ch <- UIMessageChunk{Type: ChunkFinish, FinishReason: "stop"}
	close(ch)

	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	err := PipeUIMessageStreamToResponse(rec, ch)
	require.NoError(t, err)
	assert.Equal(t, 4, rec.flushCount)

	resp := rec.Result()
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "v1", resp.Header.Get("x-vercel-ai-ui-message-stream"))
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "no", resp.Header.Get("x-accel-buffering"))

	body := rec.Body.String()
	assert.Contains(t, body, "data: ")
	assert.True(t, strings.HasSuffix(body, "data: [DONE]\n\n"))
}

func TestWriteUIMessageStream(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("hi")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hello")),
	)

	rec := httptest.NewRecorder()
	err := WriteUIMessageStream(rec, result)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "text-delta")
	assert.True(t, strings.HasSuffix(body, "data: [DONE]\n\n"))
}

func TestWriteTextStream(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("hello world")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
	)

	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	err := WriteTextStream(rec, result)
	require.NoError(t, err)

	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "hello world", rec.Body.String())
	assert.Equal(t, 1, rec.flushCount)
}

func TestWriteTextStream_ReturnsStreamError(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return nil, errors.New("provider failed")
		},
	}
	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithMaxRetries(0),
	)

	err := WriteTextStream(httptest.NewRecorder(), result)
	require.Error(t, err)
	assert.ErrorContains(t, err, "provider failed")
}

func TestStreamUIMessage(t *testing.T) {
	t.Run("emits progressive text snapshots", func(t *testing.T) {
		ch := chunks(
			UIMessageChunk{Type: ChunkStart, MessageID: "msg-1"},
			TextStartChunk("t1"),
			TextDeltaChunk("t1", "hello"),
			TextDeltaChunk("t1", " world"),
			TextEndChunk("t1"),
			UIMessageChunk{Type: ChunkFinish},
		)

		messages := collectMessages(StreamUIMessage(ch))
		require.Len(t, messages, 5)
		assert.Equal(t, "msg-1", messages[0].ID)
		assert.Empty(t, messages[0].Parts)
		assertTextPart(t, messages[1], "", "streaming")
		assertTextPart(t, messages[2], "hello", "streaming")
		assertTextPart(t, messages[3], "hello world", "streaming")
		assertTextPart(t, messages[4], "hello world", "done")
	})

	t.Run("skips error chunks and continues", func(t *testing.T) {
		ch := chunks(
			UIMessageChunk{Type: ChunkError, ErrorText: "boom"},
			TextStartChunk("t1"),
			TextDeltaChunk("t1", "ok"),
		)

		messages := collectMessages(StreamUIMessage(ch, WithUIMessageReaderGenerateID(func() string { return "fallback" })))
		require.Len(t, messages, 2)
		assert.Equal(t, "fallback", messages[0].ID)
		assertTextPart(t, messages[1], "ok", "streaming")
	})
}

func TestAssembleUIMessage(t *testing.T) {
	t.Run("assembles text parts", func(t *testing.T) {
		msg, err := AssembleUIMessage(chunks(
			UIMessageChunk{Type: ChunkStart, MessageID: "msg-1"},
			TextStartChunk("t1"),
			TextDeltaChunk("t1", "hello"),
			TextEndChunk("t1"),
			UIMessageChunk{Type: ChunkFinish},
		))
		require.NoError(t, err)

		assert.Equal(t, "msg-1", msg.ID)
		assertTextPart(t, msg, "hello", "done")
	})

	t.Run("preserves reasoning provider metadata", func(t *testing.T) {
		meta := provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"sig-123"}`)}
		msg, err := AssembleUIMessage(chunks(
			UIMessageChunk{Type: ChunkStart, MessageID: "msg-1"},
			UIMessageChunk{Type: ChunkReasoningStart, ID: "r1"},
			UIMessageChunk{Type: ChunkReasoningDelta, ID: "r1", Delta: "thinking"},
			UIMessageChunk{Type: ChunkReasoningDelta, ID: "r1", ProviderMetadata: meta},
			UIMessageChunk{Type: ChunkReasoningEnd, ID: "r1"},
			UIMessageChunk{Type: ChunkFinish},
		))
		require.NoError(t, err)

		require.Len(t, msg.Parts, 1)
		rp, ok := msg.Parts[0].(ReasoningPart)
		require.True(t, ok, "expected ReasoningPart, got %T", msg.Parts[0])
		assert.Equal(t, "thinking", rp.Text)
		assert.Equal(t, meta, rp.ProviderMetadata)
	})

	t.Run("preserves file provider metadata", func(t *testing.T) {
		meta := provider.ProviderMetadata{"anthropic": json.RawMessage(`{"cacheControl":{"type":"ephemeral"}}`)}
		msg, err := AssembleUIMessage(chunks(
			UIMessageChunk{Type: ChunkStart, MessageID: "msg-1"},
			UIMessageChunk{Type: ChunkFile, URL: "https://example.com/file.png", MediaType: "image/png", ProviderMetadata: meta},
			UIMessageChunk{Type: ChunkFinish},
		))
		require.NoError(t, err)

		require.Len(t, msg.Parts, 1)
		fp, ok := msg.Parts[0].(FilePart)
		require.True(t, ok, "expected FilePart, got %T", msg.Parts[0])
		assert.Equal(t, "https://example.com/file.png", fp.URL)
		assert.Equal(t, "image/png", fp.MediaType)
		assert.Equal(t, meta, fp.ProviderMetadata)
	})

	t.Run("assembles tool invocations", func(t *testing.T) {
		msg, err := AssembleUIMessage(chunks(
			UIMessageChunk{Type: ChunkStart, MessageID: "msg-1"},
			UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "c1", ToolName: "weather", Input: json.RawMessage(`{"city":"NYC"}`)},
			UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "c1", Output: json.RawMessage(`{"temp":72}`)},
			UIMessageChunk{Type: ChunkFinish},
		))
		require.NoError(t, err)

		assert.Equal(t, "msg-1", msg.ID)
		require.Len(t, msg.Parts, 1)
		tip, ok := msg.Parts[0].(ToolInvocationPart)
		require.True(t, ok, "expected ToolInvocationPart, got %T", msg.Parts[0])
		assert.Equal(t, ToolStateOutputAvailable, tip.State)
		assert.Equal(t, "weather", tip.ToolName)
		assert.JSONEq(t, `{"temp":72}`, string(tip.Output))
	})
}

func chunks(values ...UIMessageChunk) <-chan UIMessageChunk {
	ch := make(chan UIMessageChunk, len(values))
	for _, value := range values {
		ch <- value
	}
	close(ch)
	return ch
}

func collectMessages(ch <-chan UIMessage) []UIMessage {
	var messages []UIMessage
	for msg := range ch {
		messages = append(messages, msg)
	}
	return messages
}

func assertTextPart(t *testing.T, msg UIMessage, text, state string) {
	t.Helper()
	require.NotEmpty(t, msg.Parts)
	tp, ok := msg.Parts[0].(TextPart)
	require.True(t, ok, "expected TextPart, got %T", msg.Parts[0])
	assert.Equal(t, text, tp.Text)
	assert.Equal(t, state, tp.State)
}

func TestHTTPRoundTrip(t *testing.T) {
	original := []UIMessage{
		{
			ID:    "m1",
			Role:  RoleUser,
			Parts: []Part{TextPart{Text: "hello"}},
		},
		{
			ID:   "m2",
			Role: RoleAssistant,
			Parts: []Part{
				TextPart{Text: "hi there"},
				ToolInvocationPart{
					ToolCallID: "c1",
					ToolName:   "weather",
					State:      ToolStateOutputAvailable,
					Input:      json.RawMessage(`{"city":"NYC"}`),
					Output:     json.RawMessage(`{"temp":72}`),
				},
			},
		},
	}

	b, err := json.Marshal(original)
	require.NoError(t, err)

	var parsed []UIMessage
	require.NoError(t, json.Unmarshal(b, &parsed))

	require.Len(t, parsed, 2)
	assert.Equal(t, "tool-weather", parsed[1].Parts[1].PartType())
}
