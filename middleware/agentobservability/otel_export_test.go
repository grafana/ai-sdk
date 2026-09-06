package agentobservability

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type otelExportContextKey struct{}

type otelFlushCounter struct {
	calls atomic.Int32
}

type otelSamplingCapture struct {
	attributes chan map[string]attribute.Value
}

func (*otelFlushCounter) OnStart(context.Context, sdktrace.ReadWriteSpan) {}
func (*otelFlushCounter) OnEnd(sdktrace.ReadOnlySpan)                     {}
func (c *otelFlushCounter) Shutdown(context.Context) error                { return nil }
func (c *otelFlushCounter) ForceFlush(context.Context) error {
	c.calls.Add(1)
	return nil
}

func (s *otelSamplingCapture) ShouldSample(params sdktrace.SamplingParameters) sdktrace.SamplingResult {
	attrs := make(map[string]attribute.Value, len(params.Attributes))
	for _, attr := range params.Attributes {
		attrs[string(attr.Key)] = attr.Value
	}
	select {
	case s.attributes <- attrs:
	default:
	}
	return sdktrace.SamplingResult{Decision: sdktrace.RecordAndSample}
}

func (*otelSamplingCapture) Description() string { return "capture OTel generation start attributes" }

func newOTelExportTestClient(
	t *testing.T,
	captureMode agento11y.ContentCaptureMode,
	providerOptions ...sdktrace.TracerProviderOption,
) (*agento11y.Client, *tracetest.SpanRecorder, *sdktrace.TracerProvider, *otelFlushCounter, func()) {
	t.Helper()

	spanRecorder := tracetest.NewSpanRecorder()
	flushCounter := &otelFlushCounter{}
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithSpanProcessor(spanRecorder),
		sdktrace.WithSpanProcessor(flushCounter),
	}
	options = append(options, providerOptions...)
	tracerProvider := sdktrace.NewTracerProvider(options...)
	cfg := agento11y.DefaultConfig()
	cfg.EnableExperimentalFeatures = agento11y.BoolPtr(true)
	cfg.GenerationExport.Protocol = agento11y.GenerationExportProtocolOTel
	cfg.ContentCapture = captureMode
	cfg.TracerProvider = tracerProvider
	cfg.Flusher = tracerProvider
	client := agento11y.NewClient(cfg)

	closed := false
	shutdown := func() {
		t.Helper()
		if closed {
			return
		}
		closed = true
		require.NoError(t, client.Shutdown(context.Background()))
		assert.Positive(t, flushCounter.calls.Load(), "client shutdown must invoke the configured flusher before tracer-provider shutdown")
		require.NoError(t, tracerProvider.Shutdown(context.Background()))
	}
	t.Cleanup(shutdown)
	return client, spanRecorder, tracerProvider, flushCounter, shutdown
}

func TestConformance_OTelGenerationExport_Unary(t *testing.T) {
	client, spanRecorder, tracerProvider, flushCounter, shutdown := newOTelExportTestClient(t, agento11y.ContentCaptureModeFull)
	inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens := 120, 42, 20, 10
	expectedResult := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "routed answer"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage: provider.Usage{
			InputTokens: provider.InputTokenUsage{
				Total: &inputTokens, CacheRead: &cacheReadTokens, CacheWrite: &cacheWriteTokens,
			},
			OutputTokens: provider.OutputTokenUsage{Total: &outputTokens},
		},
		Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
			ID: "response-1", Provider: "anthropic", ModelID: "claude-sonnet-4-5",
		}},
	}
	var providerSpan trace.SpanContext
	var providerContextValue any
	model := &mockLanguageModel{
		provider_: "grafana",
		modelID:   "router-model",
		doGenerate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			providerSpan = trace.SpanFromContext(ctx).SpanContext()
			providerContextValue = ctx.Value(otelExportContextKey{})
			return expectedResult, nil
		},
	}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
			ClientResolver: func(context.Context) *agento11y.Client { return client },
		})},
	})

	maxTokens := 256
	temperature := 0.2
	topP := 0.9
	ctx := context.WithValue(context.Background(), otelExportContextKey{}, "application-value")
	ctx, callerSpan := tracerProvider.Tracer("otel-export-test").Start(ctx, "caller")
	ctx = WithGenerationID(ctx, "generation-child")
	ctx = WithParentGenerationIDs(ctx, "generation-parent")
	result, err := wrapped.DoGenerate(ctx, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("follow the system instruction"),
			provider.UserText("routed question"),
		},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeFunction, Name: "lookup", Description: "Look up a value",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		MaxOutputTokens: &maxTokens,
		Temperature:     &temperature,
		TopP:            &topP,
	})
	require.NoError(t, err)
	assert.Same(t, expectedResult, result)
	assert.Equal(t, "application-value", providerContextValue)
	assert.True(t, providerSpan.IsValid())
	assert.NotEqual(t, callerSpan.SpanContext().SpanID(), providerSpan.SpanID())
	assert.Equal(t, callerSpan.SpanContext().TraceID(), providerSpan.TraceID())
	assert.Zero(t, flushCounter.calls.Load(), "recording must not flush the application tracer provider per call")

	ended := spanRecorder.Ended()
	require.Len(t, ended, 1)
	generationSpan := testkit.FindSpan(t, ended, "chat claude-sonnet-4-5")
	assert.Equal(t, trace.SpanKindClient, generationSpan.SpanKind())
	assert.Equal(t, callerSpan.SpanContext().SpanID(), generationSpan.Parent().SpanID())
	assert.Equal(t, providerSpan.SpanID(), generationSpan.SpanContext().SpanID())

	attrs := testkit.SpanAttributes(generationSpan)
	assert.Equal(t, "true", attrs["agento11y.record"].AsString())
	assert.Equal(t, "generation-child", attrs["agento11y.generation.id"].AsString())
	assert.Equal(t, "chat", attrs["gen_ai.operation.name"].AsString())
	assert.Equal(t, "anthropic", attrs["gen_ai.provider.name"].AsString())
	assert.Equal(t, "claude-sonnet-4-5", attrs["gen_ai.request.model"].AsString())
	assert.Equal(t, "claude-sonnet-4-5", attrs["gen_ai.response.model"].AsString())
	assert.Equal(t, "response-1", attrs["gen_ai.response.id"].AsString())
	assert.Equal(t, []string{"end_turn"}, attrs["gen_ai.response.finish_reasons"].AsStringSlice())
	assert.Equal(t, int64(256), attrs["gen_ai.request.max_tokens"].AsInt64())
	assert.InDelta(t, 0.2, attrs["gen_ai.request.temperature"].AsFloat64(), 1e-9)
	assert.InDelta(t, 0.9, attrs["gen_ai.request.top_p"].AsFloat64(), 1e-9)
	assert.Contains(t, attrs["gen_ai.tool.definitions"].AsString(), "lookup")
	assert.Equal(t, "inclusive", attrs["gen_ai.token.semantics"].AsString())
	assert.Equal(t, int64(120), attrs["gen_ai.usage.input_tokens"].AsInt64())
	assert.Equal(t, int64(42), attrs["gen_ai.usage.output_tokens"].AsInt64())
	assert.Equal(t, int64(20), attrs["gen_ai.usage.cache_read.input_tokens"].AsInt64())
	assert.Equal(t, int64(10), attrs["gen_ai.usage.cache_creation.input_tokens"].AsInt64())
	assert.Equal(t, []string{"generation-parent"}, attrs["agento11y.generation.parent_generation_ids"].AsStringSlice())
	assert.Contains(t, attrs["gen_ai.system_instructions"].AsString(), "follow the system instruction")
	assert.Contains(t, attrs["gen_ai.input.messages"].AsString(), "routed question")
	assert.Contains(t, attrs["gen_ai.output.messages"].AsString(), "routed answer")

	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(attrs["agento11y.generation.metadata"].AsString()), &metadata))
	assert.Equal(t, "grafana", metadata[transportProviderMetadataKey])
	assert.Equal(t, "router-model", metadata[transportModelMetadataKey])

	callerSpan.End()
	shutdown()
}

func TestConformance_OTelGenerationExport_ImmediateErrorRetainsRequest(t *testing.T) {
	tests := []struct {
		name        string
		stream      bool
		captureMode agento11y.ContentCaptureMode
		wantContent bool
	}{
		{name: "unary", captureMode: agento11y.ContentCaptureModeFull, wantContent: true},
		{name: "stream", stream: true, captureMode: agento11y.ContentCaptureModeFull, wantContent: true},
		{name: "metadata only", captureMode: agento11y.ContentCaptureModeMetadataOnly},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sampler := &otelSamplingCapture{attributes: make(chan map[string]attribute.Value, 1)}
			client, spanRecorder, _, _, shutdown := newOTelExportTestClient(
				t,
				tc.captureMode,
				sdktrace.WithSampler(sampler),
			)
			callErr := errors.New("provider failed before returning a result")
			model := &mockLanguageModel{
				provider_: "anthropic",
				modelID:   "claude-error",
				doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return nil, callErr
				},
				doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return nil, callErr
				},
			}
			wrapped := middleware.Wrap(middleware.WrapOptions{
				Model: model,
				Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
					ClientResolver: func(context.Context) *agento11y.Client { return client },
				})},
			})

			maxTokens := 256
			temperature := 0.2
			topP := 0.9
			params := provider.CallOptions{
				Prompt: []provider.Message{
					provider.NewSystemMessage("system instruction"),
					provider.UserText("user input"),
				},
				Tools: []provider.Tool{{
					Type: provider.ToolTypeFunction, Name: "lookup",
					InputSchema: json.RawMessage(`{"type":"object"}`),
				}},
				MaxOutputTokens: &maxTokens,
				Temperature:     &temperature,
				TopP:            &topP,
			}

			var err error
			if tc.stream {
				_, err = wrapped.DoStream(context.Background(), params)
			} else {
				_, err = wrapped.DoGenerate(context.Background(), params)
			}
			require.ErrorIs(t, err, callErr)

			require.Len(t, sampler.attributes, 1)
			startAttrs := <-sampler.attributes
			assert.Equal(t, int64(256), startAttrs["gen_ai.request.max_tokens"].AsInt64())
			assert.InDelta(t, 0.2, startAttrs["gen_ai.request.temperature"].AsFloat64(), 1e-9)
			assert.InDelta(t, 0.9, startAttrs["gen_ai.request.top_p"].AsFloat64(), 1e-9)

			span := testkit.FindSpan(t, spanRecorder.Ended(), "chat claude-error")
			attrs := testkit.SpanAttributes(span)
			assert.Equal(t, codes.Error, span.Status().Code)
			assert.Equal(t, "provider_call_error", attrs["error.type"].AsString())
			for _, key := range []string{
				"gen_ai.response.id",
				"gen_ai.response.model",
				"gen_ai.response.finish_reasons",
			} {
				assert.NotContains(t, attrs, key)
			}
			assert.Equal(t, int64(256), attrs["gen_ai.request.max_tokens"].AsInt64())
			assert.InDelta(t, 0.2, attrs["gen_ai.request.temperature"].AsFloat64(), 1e-9)
			assert.InDelta(t, 0.9, attrs["gen_ai.request.top_p"].AsFloat64(), 1e-9)
			contentKeys := []string{
				"gen_ai.system_instructions",
				"gen_ai.input.messages",
				"gen_ai.tool.definitions",
			}
			if tc.wantContent {
				assert.Contains(t, attrs[contentKeys[0]].AsString(), "system instruction")
				assert.Contains(t, attrs[contentKeys[1]].AsString(), "user input")
				assert.Contains(t, attrs[contentKeys[2]].AsString(), "lookup")
			} else {
				for _, key := range contentKeys {
					assert.NotContains(t, attrs, key)
				}
			}
			shutdown()
		})
	}
}

func TestConformance_OTelGenerationExport_ProviderNames(t *testing.T) {
	tests := []struct {
		name             string
		providerName     string
		responseProvider string
		want             string
	}{
		{name: "Bedrock start identity", providerName: "amazon-bedrock", want: "aws.bedrock"},
		{name: "Anthropic Vertex response identity", providerName: "grafana", responseProvider: "anthropic.vertex", want: "gcp.vertex_ai"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, spanRecorder, _, _, shutdown := newOTelExportTestClient(t, agento11y.ContentCaptureModeMetadataOnly)
			result := &provider.GenerateResult{}
			if tc.responseProvider != "" {
				result.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
					Provider: tc.responseProvider,
					ModelID:  "model",
				}}
			}
			model := &mockLanguageModel{
				provider_: tc.providerName,
				modelID:   "model",
				doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return result, nil
				},
			}
			wrapped := middleware.Wrap(middleware.WrapOptions{
				Model: model,
				Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
					ClientResolver: func(context.Context) *agento11y.Client { return client },
				})},
			})

			_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			span := testkit.FindSpan(t, spanRecorder.Ended(), "chat model")
			assert.Equal(t, tc.want, testkit.SpanAttributes(span)["gen_ai.provider.name"].AsString())
			shutdown()
		})
	}
}

func TestConformance_OTelGenerationExport_Stream(t *testing.T) {
	client, spanRecorder, tracerProvider, flushCounter, shutdown := newOTelExportTestClient(t, agento11y.ContentCaptureModeMetadataOnly)
	upstream := make(chan provider.StreamPart)
	var providerSpan trace.SpanContext
	var providerContextValue any
	model := &mockLanguageModel{
		provider_: "anthropic",
		modelID:   "claude-stream",
		doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			providerSpan = trace.SpanFromContext(ctx).SpanContext()
			providerContextValue = ctx.Value(otelExportContextKey{})
			return &provider.StreamResult{Stream: upstream}, nil
		},
	}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
			ClientResolver: func(context.Context) *agento11y.Client { return client },
		})},
	})
	ctx := context.WithValue(context.Background(), otelExportContextKey{}, "application-value")
	ctx, callerSpan := tracerProvider.Tracer("otel-export-test").Start(ctx, "caller")
	result, err := wrapped.DoStream(ctx, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("secret system instruction"),
			provider.UserText("secret input"),
		},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeFunction, Name: "lookup",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, "application-value", providerContextValue)
	assert.True(t, providerSpan.IsValid())
	assert.NotEqual(t, callerSpan.SpanContext().SpanID(), providerSpan.SpanID())
	assert.Equal(t, callerSpan.SpanContext().TraceID(), providerSpan.TraceID())
	assert.Empty(t, spanRecorder.Ended(), "returning the provider stream must not end the generation")

	textPart := provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "secret output"}
	upstream <- textPart
	received := []provider.StreamPart{<-result.Stream}
	assert.Equal(t, textPart, received[0])
	assert.Empty(t, spanRecorder.Ended(), "observing output must not end the generation while the provider stream is open")

	inputTokens, outputTokens := 12, 4
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message: "upstream stream failed", StatusCode: 500,
	})
	errorPart := provider.StreamPart{
		Type: provider.PartError, APICallError: apiErr,
		Usage: &provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: &inputTokens},
			OutputTokens: provider.OutputTokenUsage{Total: &outputTokens},
		},
	}
	upstream <- errorPart
	close(upstream)
	for part := range result.Stream {
		received = append(received, part)
	}
	assert.Equal(t, []provider.StreamPart{textPart, errorPart}, received)
	require.Eventually(t, func() bool {
		return len(spanRecorder.Ended()) == 1
	}, 2*time.Second, 10*time.Millisecond)
	assert.Zero(t, flushCounter.calls.Load(), "stream completion must not flush the application tracer provider")

	generationSpan := testkit.FindSpan(t, spanRecorder.Ended(), "chat claude-stream")
	assert.Equal(t, callerSpan.SpanContext().SpanID(), generationSpan.Parent().SpanID())
	assert.Equal(t, providerSpan.SpanID(), generationSpan.SpanContext().SpanID())
	attrs := testkit.SpanAttributes(generationSpan)
	assert.True(t, attrs["gen_ai.request.stream"].AsBool())
	firstChunk, ok := attrs["gen_ai.response.time_to_first_chunk"]
	require.True(t, ok)
	assert.GreaterOrEqual(t, firstChunk.AsFloat64(), float64(0))
	assert.Equal(t, codes.Error, generationSpan.Status().Code)
	assert.Equal(t, "provider_call_error", attrs["error.type"].AsString())
	assert.Equal(t, "server_error", attrs["error.category"].AsString())
	for _, key := range []string{
		"gen_ai.system_instructions",
		"gen_ai.input.messages",
		"gen_ai.output.messages",
		"gen_ai.tool.definitions",
	} {
		assert.NotContains(t, attrs, key)
	}

	callerSpan.End()
	shutdown()
}

func TestConformance_OTelGenerationExport_StreamCancellation(t *testing.T) {
	client, spanRecorder, _, _, shutdown := newOTelExportTestClient(t, agento11y.ContentCaptureModeMetadataOnly)
	upstream := make(chan provider.StreamPart, 1)
	model := &mockLanguageModel{
		provider_: "anthropic",
		modelID:   "claude-cancelled",
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: upstream}, nil
		},
	}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
			ClientResolver: func(context.Context) *agento11y.Client { return client },
		})},
	})

	ctx, cancel := context.WithCancel(context.Background())
	result, err := wrapped.DoStream(ctx, provider.CallOptions{})
	require.NoError(t, err)
	cancel()
	select {
	case _, ok := <-result.Stream:
		assert.False(t, ok)
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not close after context cancellation")
	}
	require.Eventually(t, func() bool {
		return len(spanRecorder.Ended()) == 1
	}, 2*time.Second, 10*time.Millisecond)

	span := testkit.FindSpan(t, spanRecorder.Ended(), "chat claude-cancelled")
	attrs := testkit.SpanAttributes(span)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Equal(t, "provider_call_error", attrs["error.type"].AsString())
	assert.Equal(t, "timeout", attrs["error.category"].AsString())

	upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "late"}
	assert.Len(t, upstream, 1, "cancellation must not start a detached upstream drain")
	shutdown()
}
