package agentobservability

import (
	"context"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	env := testkit.NewEnv(t, func(config *agento11y.Config) {
		config.Tracer = panicTracer{}
	})
	parts := []provider.StreamPart{
		{Type: provider.PartTextDelta, Delta: "hello"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}},
	}
	model := &mockLanguageModel{provider_: "grafana", modelID: "grafana/assistant",
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, len(parts))
			for _, part := range parts {
				ch <- part
			}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}
	wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
		ClientResolver: func(context.Context) *agento11y.Client { return env.Client },
	})}})
	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.NoError(t, err)
	var got []provider.StreamPart
	for part := range result.Stream {
		got = append(got, part)
	}
	assert.Equal(t, parts, got)
}
