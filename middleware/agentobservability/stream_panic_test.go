package agentobservability

import (
	"context"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type panicSpan struct{ noop.Span }

func (panicSpan) End(...trace.SpanEndOption) { panic("secret-panic-value") }

type panicTracer struct{ noop.Tracer }

func (panicTracer) Start(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, panicSpan{}
}

func TestRecordingMiddleware_FinalizePanic(t *testing.T) {
	stop := provider.FinishReason{Unified: provider.FinishReasonStop}
	for _, tc := range []struct {
		name   string
		parts  []provider.StreamPart
		cancel bool
	}{
		{
			name: "single segment",
			parts: []provider.StreamPart{
				{Type: provider.PartTextDelta, Delta: "hello"},
				{Type: provider.PartFinish, FinishReason: &stop},
			},
		},
		{
			name: "multi segment upstream close",
			parts: []provider.StreamPart{
				{Type: provider.PartTextDelta, ID: "0", Delta: "first"},
				{Type: provider.PartFinish, FinishReason: &stop},
				{Type: provider.PartTextDelta, ID: "0", Delta: "second"},
				{Type: provider.PartFinish, FinishReason: &stop},
			},
		},
		{
			name: "multi segment cancel after finish",
			parts: []provider.StreamPart{
				{Type: provider.PartTextDelta, ID: "0", Delta: "first"},
				{Type: provider.PartFinish, FinishReason: &stop},
				{Type: provider.PartTextDelta, ID: "0", Delta: "second"},
				{Type: provider.PartFinish, FinishReason: &stop},
			},
			cancel: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testkit.NewEnv(t, func(config *agento11y.Config) {
				config.Tracer = panicTracer{}
			})
			upstream := make(chan provider.StreamPart, len(tc.parts))
			for _, part := range tc.parts {
				upstream <- part
			}
			if tc.cancel {
				t.Cleanup(func() { close(upstream) })
			} else {
				close(upstream)
			}
			stream := startRecordedStream(t, env, upstream, 0)

			var got []provider.StreamPart
			for part := range stream.result.Stream {
				got = append(got, part)
				if tc.cancel && len(got) == len(tc.parts) {
					stream.cancel()
				}
			}
			assert.Equal(t, tc.parts, got)
		})
	}
}
