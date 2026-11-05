package agentobservability

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
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
	cfg.EnableExperimentalFeatures = new(true)
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
			assert.Equal(t, "anthropic", startAttrs["gen_ai.provider.name"].AsString())
			assert.Equal(t, "claude-error", startAttrs["gen_ai.request.model"].AsString())
			for _, key := range []string{"gen_ai.request.max_tokens", "gen_ai.request.temperature", "gen_ai.request.top_p"} {
				assert.NotContains(t, startAttrs, key)
			}

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

func TestConformance_OTelGenerationExport_RecordingControls(t *testing.T) {
	for _, tc := range []struct {
		name        string
		stream      bool
		callError   bool
		filterPanic bool
	}{
		{name: "unary"},
		{name: "stream", stream: true},
		{name: "unary error", callError: true},
		{name: "stream error", stream: true, callError: true},
		{name: "unary filter panic", filterPanic: true},
		{name: "stream filter panic", stream: true, filterPanic: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sampler := &otelSamplingCapture{attributes: make(chan map[string]attribute.Value, 1)}
			client, spans, _, _, shutdown := newOTelExportTestClient(t, agento11y.ContentCaptureModeFull, sdktrace.WithSampler(sampler))
			var callErr error
			if tc.callError {
				callErr = errors.New("provider failure")
			}
			var providerValue any
			model := &mockLanguageModel{
				provider_: "amazon-bedrock", modelID: "requested-model",
				doGenerate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
					providerValue = ctx.Value(otelExportContextKey{})
					if callErr != nil {
						return nil, callErr
					}
					return &provider.GenerateResult{Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{Provider: "private-provider", ModelID: "private-model", ID: "private-id"}}}, nil
				},
				doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
					providerValue = ctx.Value(otelExportContextKey{})
					if callErr != nil {
						return nil, callErr
					}
					parts := make(chan provider.StreamPart, 1)
					parts <- provider.StreamPart{Type: provider.PartResponseMeta, Provider: "private-provider", ModelID: "private-model", ResponseID: "private-id"}
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				},
			}
			filterCalls := 0
			wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
				ClientResolver: func(context.Context) *agento11y.Client { return client },
				ContextSource:  ContextProvidedOnly,
				IdentitySource: IdentityRequested,
				GenerationFilter: func(input GenerationFilterInput) agento11y.Generation {
					filterCalls++
					assert.Equal(t, "bedrock", input.Generation.Model.Provider)
					assert.Empty(t, input.Generation.ResponseModel)
					assert.Empty(t, input.Generation.ResponseID)
					assert.NotContains(t, input.Generation.Metadata, transportProviderMetadataKey)
					if tc.filterPanic {
						panic("filter failed")
					}
					return agento11y.Generation{Model: input.Generation.Model}
				},
			})}})
			ctx := agento11y.WithUserID(context.Background(), "ambient-user")
			ctx = agento11y.WithTag(ctx, "private-tag", "private-value")
			ctx = context.WithValue(ctx, otelExportContextKey{}, "provider-value")
			maxTokens := 256
			params := provider.CallOptions{
				Prompt:          []provider.Message{provider.NewSystemMessage("private-system"), provider.UserText("private-input")},
				Tools:           []provider.Tool{{Type: provider.ToolTypeFunction, Name: "private-tool", InputSchema: json.RawMessage(`{"type":"object"}`)}},
				MaxOutputTokens: &maxTokens,
				ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"budgetTokens":123}}`)}},
			}
			var err error
			if tc.stream {
				var result *provider.StreamResult
				result, err = wrapped.DoStream(ctx, params)
				if result != nil {
					for range result.Stream {
					}
				}
			} else {
				_, err = wrapped.DoGenerate(ctx, params)
			}
			require.ErrorIs(t, err, callErr)
			assert.Equal(t, "provider-value", providerValue)
			assert.Equal(t, 1, filterCalls)
			require.Len(t, sampler.attributes, 1)
			assert.NotContains(t, <-sampler.attributes, "gen_ai.request.max_tokens")
			span := testkit.FindSpan(t, spans.Ended(), "chat requested-model")
			attrs := testkit.SpanAttributes(span)
			assert.Equal(t, "aws.bedrock", attrs["gen_ai.provider.name"].AsString())
			for _, key := range []string{"gen_ai.system_instructions", "gen_ai.input.messages", "gen_ai.tool.definitions", "gen_ai.request.max_tokens", "gen_ai.response.id", "gen_ai.response.model", "agento11y.gen_ai.request.thinking.budget_tokens"} {
				assert.NotContains(t, attrs, key)
			}
			encoded, err := json.Marshal(attrs)
			require.NoError(t, err)
			for _, value := range []string{"ambient-user", "private-tag", "private-value", "private-system", "private-input", "private-tool", "private-provider", "private-model", "private-id"} {
				assert.NotContains(t, string(encoded), value)
			}
			shutdown()
		})
	}
}

func TestConformance_OTelGenerationExport_ProviderNames(t *testing.T) {
	type providerNameCase struct {
		name             string
		providerName     string
		responseProvider string
		want             string
	}
	tests := []providerNameCase{
		{name: "Bedrock start identity", providerName: "amazon-bedrock", want: "aws.bedrock"},
		{name: "Anthropic Vertex response identity", providerName: "grafana", responseProvider: "anthropic.vertex", want: "gcp.vertex_ai"},
	}
	for _, stored := range slices.Sorted(maps.Keys(otelProviderNames)) {
		tests = append(tests, providerNameCase{name: "preflight table " + stored, providerName: stored, want: otelProviderName(stored)})
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
	require.Len(t, spanRecorder.Ended(), 1, "the recorder must end before downstream EOF")
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
	assert.Equal(t, "inclusive", attrs["gen_ai.token.semantics"].AsString())
	assert.Equal(t, int64(12), attrs["gen_ai.usage.input_tokens"].AsInt64())
	assert.Equal(t, int64(4), attrs["gen_ai.usage.output_tokens"].AsInt64())
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
		require.FailNow(t, "stream did not close after context cancellation")
	}
	require.Len(t, spanRecorder.Ended(), 1, "cancellation must finalize before downstream EOF")

	span := testkit.FindSpan(t, spanRecorder.Ended(), "chat claude-cancelled")
	attrs := testkit.SpanAttributes(span)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Equal(t, "provider_call_error", attrs["error.type"].AsString())
	assert.Equal(t, "timeout", attrs["error.category"].AsString())

	upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "late"}
	assert.Len(t, upstream, 1, "cancellation must not start a detached upstream drain")
	shutdown()
}
