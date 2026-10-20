package logger

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/internal/streamrecover"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func streamOf(parts ...provider.StreamPart) *mockModel {
	return &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		ch := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			ch <- part
		}
		close(ch)
		return &provider.StreamResult{Stream: ch}, nil
	}}
}

func TestDoStream_ObservationPanic(t *testing.T) {
	parts := []provider.StreamPart{
		{Type: provider.PartTextDelta, ID: "txt", Delta: "hello"},
		{Type: provider.PartTextDelta, ID: "txt", Delta: " world"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}},
	}
	var calls atomic.Int32
	clock := func() time.Time {
		if calls.Add(1) > 1 {
			panic("secret-panic-value")
		}
		return time.Unix(0, 0)
	}
	wrapped := Wrap(streamOf(parts...), Options{Logger: slog.New(newTestHandler()), Clock: clock, LogStreamParts: true})
	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assert.Equal(t, parts, drainStream(result.Stream))
}

func TestDoStream_RecoveredPanicErrorClass(t *testing.T) {
	handler := newTestHandler()
	wrapped := Wrap(streamOf(streamrecover.Part()), Options{Logger: slog.New(handler)})
	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	drainStream(result.Stream)
	records := handler.Records()
	require.NotEmpty(t, records)
	attrs := records[len(records)-1].AttrsMap()
	assertAttr(t, attrs, "ai_sdk.error.type", "internal_error")
}
