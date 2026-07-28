package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformStream(t *testing.T) {
	t.Run("OneToOne", func(t *testing.T) {
		ch := make(chan provider.StreamPart, 2)
		ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "hello"}
		ch <- provider.StreamPart{Type: provider.PartFinish}
		close(ch)

		result := &provider.StreamResult{Stream: ch}
		transformed := TransformStream(context.Background(), result,
			func(part provider.StreamPart, emit func(provider.StreamPart)) {
				if part.Type == provider.PartTextDelta {
					part.Delta = "[" + part.Delta + "]"
				}
				emit(part)
			}, nil)

		var parts []provider.StreamPart
		for p := range transformed.Stream {
			parts = append(parts, p)
		}
		require.Len(t, parts, 2)
		assert.Equal(t, "[hello]", parts[0].Delta)
		assert.Equal(t, provider.PartFinish, parts[1].Type)
	})

	t.Run("OneToMany", func(t *testing.T) {
		ch := make(chan provider.StreamPart, 1)
		ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "abc"}
		close(ch)

		result := &provider.StreamResult{Stream: ch}
		transformed := TransformStream(context.Background(), result,
			func(part provider.StreamPart, emit func(provider.StreamPart)) {
				for _, c := range part.Delta {
					emit(provider.StreamPart{Type: provider.PartTextDelta, Delta: string(c)})
				}
			}, nil)

		var parts []provider.StreamPart
		for p := range transformed.Stream {
			parts = append(parts, p)
		}
		require.Len(t, parts, 3)
		assert.Equal(t, "a", parts[0].Delta)
		assert.Equal(t, "b", parts[1].Delta)
		assert.Equal(t, "c", parts[2].Delta)
	})

	t.Run("StatefulBuffering", func(t *testing.T) {
		ch := make(chan provider.StreamPart, 3)
		ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "a"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "b"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "c"}
		close(ch)

		var buffer string
		result := &provider.StreamResult{Stream: ch}
		transformed := TransformStream(context.Background(), result,
			func(part provider.StreamPart, emit func(provider.StreamPart)) {
				buffer += part.Delta
			},
			func(emit func(provider.StreamPart)) {
				emit(provider.StreamPart{Type: provider.PartTextDelta, Delta: buffer})
			},
		)

		var parts []provider.StreamPart
		for p := range transformed.Stream {
			parts = append(parts, p)
		}
		require.Len(t, parts, 1)
		assert.Equal(t, "abc", parts[0].Delta)
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ch := make(chan provider.StreamPart)

		ctx, cancel := context.WithCancel(context.Background())
		result := &provider.StreamResult{Stream: ch}
		transformed := TransformStream(ctx, result,
			func(part provider.StreamPart, emit func(provider.StreamPart)) {
				emit(part)
			}, nil)

		cancel()

		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case _, ok := <-transformed.Stream:
			assert.False(t, ok, "stream should close after context cancellation")
		case <-timer.C:
			t.Fatal("timed out waiting for stream to close after context cancellation")
		}
	})

	t.Run("EmptyStream", func(t *testing.T) {
		ch := make(chan provider.StreamPart)
		close(ch)

		result := &provider.StreamResult{Stream: ch}
		transformed := TransformStream(context.Background(), result,
			func(part provider.StreamPart, emit func(provider.StreamPart)) {
				emit(part)
			}, nil)

		var parts []provider.StreamPart
		for p := range transformed.Stream {
			parts = append(parts, p)
		}
		assert.Empty(t, parts)
	})

	t.Run("PreservesMetadata", func(t *testing.T) {
		ch := make(chan provider.StreamPart)
		close(ch)

		req := &provider.RequestMetadata{}
		resp := &provider.ResponseHeaders{Headers: map[string]string{"x-foo": "bar"}}
		result := &provider.StreamResult{Stream: ch, Request: req, Response: resp}
		transformed := TransformStream(context.Background(), result,
			func(part provider.StreamPart, emit func(provider.StreamPart)) {
				emit(part)
			}, nil)

		assert.Equal(t, req, transformed.Request)
		assert.Equal(t, resp, transformed.Response)
	})
}
