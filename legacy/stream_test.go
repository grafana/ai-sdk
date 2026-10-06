package aisdk

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIMessageStreamWriter(t *testing.T) {
	t.Run("write and close", func(t *testing.T) {
		w := newUIMessageStreamWriter(16)

		require.NoError(t, w.Write(TextDeltaChunk("b1", "hello")))
		require.NoError(t, w.Close())

		chunk := <-w.output()
		assert.Equal(t, ChunkTextDelta, chunk.Type)
		assert.Equal(t, "hello", chunk.Delta)

		_, ok := <-w.output()
		assert.False(t, ok, "expected channel to be closed")
	})

	t.Run("write after close returns ErrWriterClosed", func(t *testing.T) {
		w := newUIMessageStreamWriter(16)
		_ = w.Close()
		err := w.Write(TextDeltaChunk("b1", "x"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrWriterClosed))
	})

	t.Run("merge forwards chunks from source", func(t *testing.T) {
		w := newUIMessageStreamWriter(16)
		src := make(chan UIMessageChunk, 3)
		src <- TextDeltaChunk("b1", "a")
		src <- TextDeltaChunk("b1", "b")
		src <- TextDeltaChunk("b1", "c")
		close(src)

		go func() {
			_ = w.Merge(src)
			_ = w.Close()
		}()

		var chunks []UIMessageChunk
		for c := range w.output() {
			chunks = append(chunks, c)
		}
		assert.Len(t, chunks, 3)
	})
}

func TestCreateUIMessageStream(t *testing.T) {
	t.Run("basic execute produces start and finish", func(t *testing.T) {
		stream := CreateUIMessageStream(CreateUIMessageStreamParams{
			Execute: func(w *UIMessageStreamWriter) error {
				_ = w.Write(TextDeltaChunk("b1", "hello"))
				_ = w.Write(TextDeltaChunk("b1", " world"))
				return nil
			},
		})

		var chunks []UIMessageChunk
		for c := range stream {
			chunks = append(chunks, c)
		}

		require.Len(t, chunks, 4) // start + 2 deltas + finish
		assert.Equal(t, ChunkStart, chunks[0].Type)
		assert.Equal(t, ChunkFinish, chunks[3].Type)
	})

	t.Run("error is masked by default", func(t *testing.T) {
		stream := CreateUIMessageStream(CreateUIMessageStreamParams{
			Execute: func(w *UIMessageStreamWriter) error {
				var v any
				return json.Unmarshal([]byte("bad"), &v)
			},
		})

		var errChunk *UIMessageChunk
		for c := range stream {
			if c.Type == ChunkError {
				errChunk = &c
			}
		}
		require.NotNil(t, errChunk)
		assert.Equal(t, "An error occurred", errChunk.ErrorText)
	})

	t.Run("OnError callback customizes error message", func(t *testing.T) {
		stream := CreateUIMessageStream(CreateUIMessageStreamParams{
			Execute: func(w *UIMessageStreamWriter) error {
				var v any
				return json.Unmarshal([]byte("bad"), &v)
			},
			OnError: func(err error) string {
				return "custom: " + err.Error()
			},
		})

		var errChunk *UIMessageChunk
		for c := range stream {
			if c.Type == ChunkError {
				errChunk = &c
			}
		}
		require.NotNil(t, errChunk)
		assert.NotEqual(t, "An error occurred", errChunk.ErrorText)
	})

	t.Run("persistence mode sets messageId on start", func(t *testing.T) {
		stream := CreateUIMessageStream(CreateUIMessageStreamParams{
			Execute: func(w *UIMessageStreamWriter) error {
				_ = w.Write(TextDeltaChunk("b1", "hi"))
				return nil
			},
			OriginalMessages: []UIMessage{{ID: "msg-0", Role: RoleUser}},
		})

		var startChunk UIMessageChunk
		for c := range stream {
			if c.Type == ChunkStart {
				startChunk = c
			}
		}
		assert.NotEmpty(t, startChunk.MessageID)
	})

	t.Run("OnFinish receives assembled messages", func(t *testing.T) {
		var finishState UIMessageStreamOnFinishState
		stream := CreateUIMessageStream(CreateUIMessageStreamParams{
			Execute: func(w *UIMessageStreamWriter) error {
				_ = w.Write(TextStartChunk("b1"))
				_ = w.Write(TextDeltaChunk("b1", "hello"))
				_ = w.Write(TextEndChunk("b1"))
				return nil
			},
			OriginalMessages: []UIMessage{{ID: "msg-0", Role: RoleUser}},
			OnFinish: func(state UIMessageStreamOnFinishState) {
				finishState = state
			},
		})
		for range stream {
		}

		assert.Len(t, finishState.Messages, 2)
		assert.NotEmpty(t, finishState.ResponseMessage.ID)
	})
}

func TestTransientDataExcludedFromAssembly(t *testing.T) {
	var finishState UIMessageStreamOnFinishState
	stream := CreateUIMessageStream(CreateUIMessageStreamParams{
		Execute: func(w *UIMessageStreamWriter) error {
			_ = w.Write(DataChunk("status", json.RawMessage(`{"v":1}`), true))  // transient
			_ = w.Write(DataChunk("result", json.RawMessage(`{"v":2}`), false)) // persistent
			return nil
		},
		OriginalMessages: []UIMessage{},
		OnFinish: func(state UIMessageStreamOnFinishState) {
			finishState = state
		},
	})
	for range stream {
	}

	dataParts := 0
	for _, p := range finishState.ResponseMessage.Parts {
		if _, ok := p.(DataPart); ok {
			dataParts++
		}
	}
	assert.Equal(t, 1, dataParts, "only non-transient data parts should be assembled")
}

func TestDataReconciliationByID(t *testing.T) {
	var finishState UIMessageStreamOnFinishState
	stream := CreateUIMessageStream(CreateUIMessageStreamParams{
		Execute: func(w *UIMessageStreamWriter) error {
			_ = w.Write(UIMessageChunk{DataName: "x", ID: "d1", Data: json.RawMessage(`{"v":1}`)})
			_ = w.Write(UIMessageChunk{DataName: "x", ID: "d1", Data: json.RawMessage(`{"v":2}`)})
			return nil
		},
		OriginalMessages: []UIMessage{},
		OnFinish: func(state UIMessageStreamOnFinishState) {
			finishState = state
		},
	})
	for range stream {
	}

	dataParts := 0
	for _, p := range finishState.ResponseMessage.Parts {
		if dp, ok := p.(DataPart); ok {
			dataParts++
			assert.JSONEq(t, `{"v":2}`, string(dp.Data))
		}
	}
	assert.Equal(t, 1, dataParts, "duplicate IDs should be reconciled")
}

func TestSendOptionDefaults(t *testing.T) {
	assert.True(t, sendOption(nil, true), "nil should use default true")
	assert.False(t, sendOption(nil, false), "nil should use default false")

	bTrue := true
	assert.True(t, sendOption(&bTrue, false), "explicit true should override default")

	bFalse := false
	assert.False(t, sendOption(&bFalse, true), "explicit false should override default")
}

func TestFilterChunks(t *testing.T) {
	makeStream := func() chan UIMessageChunk {
		in := make(chan UIMessageChunk, 4)
		in <- UIMessageChunk{Type: ChunkStart}
		in <- UIMessageChunk{Type: ChunkSourceURL, SourceID: "s1"}
		in <- UIMessageChunk{Type: ChunkTextDelta, Delta: "hi"}
		in <- UIMessageChunk{Type: ChunkFinish}
		close(in)
		return in
	}

	t.Run("sources filtered by default", func(t *testing.T) {
		out := filterChunks(makeStream(), uiMessageStreamConfig{})
		for c := range out {
			assert.NotEqual(t, ChunkSourceURL, c.Type, "source chunk should be filtered by default")
		}
	})

	t.Run("sources pass through when enabled", func(t *testing.T) {
		sendSources := true
		out := filterChunks(makeStream(), uiMessageStreamConfig{sendSources: &sendSources})
		var hasSource bool
		for c := range out {
			if c.Type == ChunkSourceURL {
				hasSource = true
			}
		}
		assert.True(t, hasSource, "source chunk should pass through when SendSources=true")
	})

	t.Run("finish filtered when disabled", func(t *testing.T) {
		in := make(chan UIMessageChunk, 3)
		in <- UIMessageChunk{Type: ChunkStart}
		in <- UIMessageChunk{Type: ChunkTextDelta, Delta: "hi"}
		in <- UIMessageChunk{Type: ChunkFinish}
		close(in)

		sendFinish := false
		out := filterChunks(in, uiMessageStreamConfig{sendFinish: &sendFinish})
		for c := range out {
			assert.NotEqual(t, ChunkFinish, c.Type)
		}
	})

	t.Run("start filtered when disabled", func(t *testing.T) {
		in := make(chan UIMessageChunk, 3)
		in <- UIMessageChunk{Type: ChunkStart}
		in <- UIMessageChunk{Type: ChunkTextDelta, Delta: "hi"}
		in <- UIMessageChunk{Type: ChunkFinish}
		close(in)

		sendStart := false
		out := filterChunks(in, uiMessageStreamConfig{sendStart: &sendStart})
		for c := range out {
			assert.NotEqual(t, ChunkStart, c.Type)
		}
	})
}

func TestAssembleResponseMessage(t *testing.T) {
	t.Run("multi-block text", func(t *testing.T) {
		chunks := []UIMessageChunk{
			{Type: ChunkTextStart, ID: "t1"},
			{Type: ChunkTextDelta, ID: "t1", Delta: "Hello "},
			{Type: ChunkTextDelta, ID: "t1", Delta: "world"},
			{Type: ChunkTextEnd, ID: "t1"},
			{Type: ChunkTextStart, ID: "t2"},
			{Type: ChunkTextDelta, ID: "t2", Delta: "Second block"},
			{Type: ChunkTextEnd, ID: "t2"},
		}

		msg := assembleResponseMessage("msg-1", chunks)
		require.Len(t, msg.Parts, 2)

		tp1, ok := msg.Parts[0].(TextPart)
		require.True(t, ok, "expected TextPart at index 0")
		assert.Equal(t, "Hello world", tp1.Text)

		tp2, ok := msg.Parts[1].(TextPart)
		require.True(t, ok, "expected TextPart at index 1")
		assert.Equal(t, "Second block", tp2.Text)
	})

	t.Run("multi-block reasoning", func(t *testing.T) {
		chunks := []UIMessageChunk{
			{Type: ChunkReasoningStart, ID: "r1"},
			{Type: ChunkReasoningDelta, ID: "r1", Delta: "Thinking..."},
			{Type: ChunkReasoningEnd, ID: "r1"},
			{Type: ChunkReasoningStart, ID: "r2"},
			{Type: ChunkReasoningDelta, ID: "r2", Delta: "More thinking"},
			{Type: ChunkReasoningEnd, ID: "r2"},
			{Type: ChunkTextStart, ID: "t1"},
			{Type: ChunkTextDelta, ID: "t1", Delta: "Answer"},
			{Type: ChunkTextEnd, ID: "t1"},
		}

		msg := assembleResponseMessage("msg-1", chunks)
		require.Len(t, msg.Parts, 3)

		rp1, ok := msg.Parts[0].(ReasoningPart)
		require.True(t, ok, "expected ReasoningPart at 0")
		assert.Equal(t, "Thinking...", rp1.Text)

		rp2, ok := msg.Parts[1].(ReasoningPart)
		require.True(t, ok, "expected ReasoningPart at 1")
		assert.Equal(t, "More thinking", rp2.Text)

		tp, ok := msg.Parts[2].(TextPart)
		require.True(t, ok, "expected TextPart at 2")
		assert.Equal(t, "Answer", tp.Text)
	})

	t.Run("tool input error", func(t *testing.T) {
		chunks := []UIMessageChunk{
			{Type: ChunkToolInputError, ToolCallID: "c1", ToolName: "calc", Input: json.RawMessage(`{"x":1}`), ErrorText: "invalid input"},
		}
		msg := assembleResponseMessage("msg-1", chunks)
		require.Len(t, msg.Parts, 1)

		tip, ok := msg.Parts[0].(ToolInvocationPart)
		require.True(t, ok, "expected ToolInvocationPart")
		assert.Equal(t, ToolStateOutputError, tip.State)
		assert.Equal(t, "invalid input", tip.ErrorText)
	})
}

func TestSSEFormat(t *testing.T) {
	b, err := FormatSSEEvent(TextDeltaChunk("t1", "hi"))
	require.NoError(t, err)
	got := string(b)
	assert.True(t, len(got) > 6 && got[:6] == "data: ", "expected SSE prefix")
	assert.True(t, got[len(got)-2:] == "\n\n", "expected double newline terminator")
}
