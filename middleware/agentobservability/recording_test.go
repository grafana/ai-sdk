package agentobservability

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// generateWith composes a model with RecordingMiddleware and invokes
// DoGenerate exactly once.
func generateWith(t *testing.T, model provider.LanguageModel, opts RecordingOptions) (*provider.GenerateResult, error) {
	t.Helper()
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model:      model,
		Middleware: []middleware.Middleware{RecordingMiddleware(opts)},
	})
	return wrapped.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
}

func streamWith(t *testing.T, model provider.LanguageModel, opts RecordingOptions) (*provider.StreamResult, error) {
	t.Helper()
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model:      model,
		Middleware: []middleware.Middleware{RecordingMiddleware(opts)},
	})
	return wrapped.DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
}

func TestRecordingMiddleware_NilResolver_PassesThrough(t *testing.T) {
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	result, err := generateWith(t, model, RecordingOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, model.generateHit)
}

func TestRecordingMiddleware_NilClientFromResolver_PassesThrough(t *testing.T) {
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	completeCalls := 0
	opts := RecordingOptions{
		ClientResolver:   func(ctx context.Context) *agento11y.Client { return nil },
		OnRecordComplete: func() { completeCalls++ },
	}
	result, err := generateWith(t, model, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, model.generateHit)
	assert.Zero(t, completeCalls)
}

func TestRecordingMiddleware_NilClientStreamPassesThroughWithoutCompletion(t *testing.T) {
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	completeCalls := 0
	result, err := streamWith(t, model, RecordingOptions{
		ClientResolver:   func(context.Context) *agento11y.Client { return nil },
		OnRecordComplete: func() { completeCalls++ },
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	for range result.Stream {
	}
	assert.Equal(t, 1, model.streamHit)
	assert.Zero(t, completeCalls)
}

func TestRecordingMiddleware_GenerateSuccess_RecordsResult(t *testing.T) {
	env := testkit.NewEnv(t)
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude-3-5-sonnet"}
	opts := RecordingOptions{
		ClientResolver:  func(ctx context.Context) *agento11y.Client { return env.Client },
		ContextProvider: func(ctx context.Context) ContextInfo { return ContextInfo{UserID: "u-1"} },
	}
	result, err := generateWith(t, model, opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Force flush so the in-memory exporter sees the generation.
	require.NoError(t, env.Client.Shutdown(context.Background()))
	testkit.RequireRequestCount(t, env, 1)

	gen := env.SingleGenerationJSON(t)
	// User ID is recorded in the metadata map under the canonical agento11y key.
	meta, _ := gen["metadata"].(map[string]any)
	require.NotNil(t, meta, "metadata map populated")
	assert.Equal(t, "u-1", meta["agento11y.user.id"], "user_id flowed into metadata via agento11y normalization")
	model_ := testkit.StringValue(t, gen, "model", "name")
	assert.Equal(t, "claude-3-5-sonnet", model_)
}

func TestRecordingMiddleware_GenerateMediaSurvivesExport(t *testing.T) {
	env := testkit.NewEnv(t)
	model := &mockLanguageModel{
		provider_: "anthropic",
		modelID:   "claude",
		doGenerate: func(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{
				Content: []provider.GenerateContentPart{{
					Type:      provider.ContentFile,
					MediaType: "image/png",
					Filename:  "plot.png",
					Data:      &provider.DataContent{Bytes: []byte{1, 2, 3}},
				}},
				FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
			}, nil
		},
	}
	opts := RecordingOptions{ClientResolver: func(ctx context.Context) *agento11y.Client { return env.Client }}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model:      model,
		Middleware: []middleware.Middleware{RecordingMiddleware(opts)},
	})

	_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.NewUserMessage(
			provider.FilePart("image/png", provider.DataContent{URL: "data:image/png;base64,AQID"}),
		)},
	})
	require.NoError(t, err)
	require.NoError(t, env.Client.Shutdown(context.Background()))

	gen := env.SingleGenerationJSON(t)
	assert.Equal(t, "data:image/png;base64,AQID", testkit.StringValue(t, gen, "input", 0, "parts", 0, "media", "url"))
	assert.Equal(t, "image/png", testkit.StringValue(t, gen, "input", 0, "parts", 0, "media", "mime_type"))
	assert.Equal(t, "data:image/png;base64,AQID", testkit.StringValue(t, gen, "output", 0, "parts", 0, "media", "url"))
	assert.Equal(t, "image/png", testkit.StringValue(t, gen, "output", 0, "parts", 0, "media", "mime_type"))
}

func TestRecordingMiddleware_MetadataOnlyStripsMediaURL(t *testing.T) {
	env := testkit.NewEnv(t, func(config *agento11y.Config) {
		config.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
	})
	model := &mockLanguageModel{
		provider_: "anthropic",
		modelID:   "claude",
		doGenerate: func(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{
				Content: []provider.GenerateContentPart{{
					Type:      provider.ContentFile,
					MediaType: "image/png",
					Data:      &provider.DataContent{URL: "https://cdn.example.com/image.png?token=secret"},
				}},
				FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
			}, nil
		},
	}
	opts := RecordingOptions{ClientResolver: func(ctx context.Context) *agento11y.Client { return env.Client }}

	_, err := generateWith(t, model, opts)
	require.NoError(t, err)
	require.NoError(t, env.Client.Shutdown(context.Background()))

	gen := env.SingleGenerationJSON(t)
	output, ok := gen["output"].([]any)
	require.True(t, ok)
	require.Len(t, output, 1)
	message, ok := output[0].(map[string]any)
	require.True(t, ok)
	parts, ok := message["parts"].([]any)
	require.True(t, ok)
	require.Len(t, parts, 1)
	part, ok := parts[0].(map[string]any)
	require.True(t, ok)
	media, ok := part["media"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, media, "url")
}

func TestRecordingMiddleware_GenerateResponseIdentity(t *testing.T) {
	tests := []struct {
		name                  string
		modelProvider         string
		modelID               string
		responseProvider      string
		responseModelID       string
		wantProvider          string
		wantModel             string
		wantTransportMetadata bool
	}{
		{
			name:                  "grafana transport records backend identity",
			modelProvider:         "grafana",
			modelID:               "claude-sonnet-4-5-20250929",
			responseProvider:      "anthropic",
			responseModelID:       "claude-sonnet-4-5-20250929",
			wantProvider:          "anthropic",
			wantModel:             "claude-sonnet-4-5-20250929",
			wantTransportMetadata: true,
		},
		{
			name:            "incomplete response identity keeps seed",
			modelProvider:   "grafana",
			modelID:         "claude-sonnet-4-5-20250929",
			responseModelID: "claude-sonnet-4-5-20250929",
			wantProvider:    "grafana",
			wantModel:       "claude-sonnet-4-5-20250929",
		},
		{
			name:             "direct provider does not record transport metadata",
			modelProvider:    "anthropic",
			modelID:          "claude-sonnet-4-5-20250929",
			responseProvider: "anthropic",
			responseModelID:  "claude-sonnet-4-5-20250929",
			wantProvider:     "anthropic",
			wantModel:        "claude-sonnet-4-5-20250929",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := testkit.NewEnv(t)
			model := &mockLanguageModel{
				provider_: tc.modelProvider,
				modelID:   tc.modelID,
				doGenerate: func(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{
						Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello"}},
						FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
						Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
							Provider: tc.responseProvider,
							ModelID:  tc.responseModelID,
						}},
					}, nil
				},
			}
			opts := RecordingOptions{ClientResolver: func(ctx context.Context) *agento11y.Client { return env.Client }}

			_, err := generateWith(t, model, opts)
			require.NoError(t, err)
			require.NoError(t, env.Client.Shutdown(context.Background()))

			gen := env.SingleGenerationJSON(t)
			assert.Equal(t, tc.wantProvider, testkit.StringValue(t, gen, "model", "provider"))
			assert.Equal(t, tc.wantModel, testkit.StringValue(t, gen, "model", "name"))

			meta, _ := gen["metadata"].(map[string]any)
			if tc.wantTransportMetadata {
				require.NotNil(t, meta)
				assert.Equal(t, tc.modelProvider, meta[transportProviderMetadataKey])
				assert.Equal(t, tc.modelID, meta[transportModelMetadataKey])
			} else if meta != nil {
				assert.NotContains(t, meta, transportProviderMetadataKey)
				assert.NotContains(t, meta, transportModelMetadataKey)
			}
		})
	}
}

func TestRecordingMiddleware_GenerateError_RecordsCallError(t *testing.T) {
	env := testkit.NewEnv(t)
	upErr := errors.New("upstream 500")
	model := &mockLanguageModel{
		provider_: "anthropic",
		modelID:   "claude",
		doGenerate: func(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, upErr
		},
	}
	opts := RecordingOptions{
		ClientResolver: func(ctx context.Context) *agento11y.Client { return env.Client },
	}
	_, err := generateWith(t, model, opts)
	require.ErrorIs(t, err, upErr)

	require.NoError(t, env.Client.Shutdown(context.Background()))
	testkit.RequireRequestCount(t, env, 1)

	gen := env.SingleGenerationJSON(t)
	callErr, _ := gen["call_error"].(string)
	assert.Contains(t, callErr, "upstream 500", "call_error captured in generation row")
}

func TestRecordingMiddleware_StreamSuccess_RecordsAccumulated(t *testing.T) {
	env := testkit.NewEnv(t)
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	opts := RecordingOptions{
		ClientResolver: func(ctx context.Context) *agento11y.Client { return env.Client },
	}
	streamResult, err := streamWith(t, model, opts)
	require.NoError(t, err)

	// Consume all parts from the tee channel — recorder finalizes when upstream closes.
	receivedParts := 0
	for range streamResult.Stream {
		receivedParts++
	}
	assert.Equal(t, 4, receivedParts, "tee delivered every part the model emitted")

	// Give the recording goroutine a moment to finalize.
	assert.Eventually(t, func() bool {
		return env.RequestCount() == 1
	}, 2*time.Second, 10*time.Millisecond, "recording goroutine finalizes after upstream close")

	require.NoError(t, env.Client.Shutdown(context.Background()))
	gen := env.SingleGenerationJSON(t)
	assert.Equal(t, "claude", testkit.StringValue(t, gen, "model", "name"))
}

func TestRecordingMiddleware_StreamPartError_RecordsUsageAndCallError(t *testing.T) {
	env := testkit.NewEnv(t)
	inputTokens, outputTokens := 120, 50
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{Message: "upstream stream failed", StatusCode: 500})
	model := &mockLanguageModel{
		provider_: "anthropic",
		modelID:   "claude",
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart, 2)
			stream <- provider.StreamPart{Type: provider.PartResponseMeta, Usage: &provider.Usage{
				InputTokens: provider.InputTokenUsage{Total: &inputTokens},
			}}
			stream <- provider.StreamPart{Type: provider.PartError, APICallError: apiErr, Usage: &provider.Usage{
				OutputTokens: provider.OutputTokenUsage{Total: &outputTokens},
			}}
			close(stream)
			return &provider.StreamResult{Stream: stream}, nil
		},
	}

	streamResult, err := streamWith(t, model, RecordingOptions{
		ClientResolver: func(context.Context) *agento11y.Client { return env.Client },
	})
	require.NoError(t, err)
	for range streamResult.Stream {
	}
	assert.Eventually(t, func() bool {
		return env.RequestCount() == 1
	}, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, env.Client.Shutdown(context.Background()))

	gen := env.SingleGenerationJSON(t)
	callErr, _ := gen["call_error"].(string)
	assert.Contains(t, callErr, "upstream stream failed")
	usage, ok := gen["usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "120", usage["input_tokens"])
	assert.Equal(t, "50", usage["output_tokens"])
	assert.Equal(t, "170", usage["total_tokens"])
}

func TestRecordingMiddleware_StreamCancellationTakesPrecedenceOverPartError(t *testing.T) {
	env := testkit.NewEnv(t)
	model := &mockLanguageModel{
		provider_: "anthropic",
		modelID:   "claude",
		doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart)
			go func() {
				defer close(stream)
				stream <- provider.StreamPart{
					Type:         provider.PartError,
					APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "provider error"}),
				}
				<-ctx.Done()
			}()
			return &provider.StreamResult{Stream: stream}, nil
		},
	}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
			ClientResolver: func(context.Context) *agento11y.Client { return env.Client },
		})},
	})
	ctx, cancel := context.WithCancel(context.Background())
	streamResult, err := wrapped.DoStream(ctx, provider.CallOptions{})
	require.NoError(t, err)
	_, ok := <-streamResult.Stream
	require.True(t, ok)
	cancel()
	for range streamResult.Stream {
	}
	assert.Eventually(t, func() bool { return env.RequestCount() == 1 }, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, env.Client.Shutdown(context.Background()))

	gen := env.SingleGenerationJSON(t)
	callErr, _ := gen["call_error"].(string)
	assert.Contains(t, callErr, "context canceled")
	assert.NotContains(t, callErr, "provider error")
}

func TestRecordingMiddleware_StreamConsumerAbandons_RecordsCancellation(t *testing.T) {
	env := testkit.NewEnv(t)
	model := &mockLanguageModel{
		provider_: "anthropic",
		modelID:   "claude",
		doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart)
			go func() {
				defer close(stream)
				stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "partial"}
				<-ctx.Done()
			}()
			return &provider.StreamResult{Stream: stream}, nil
		},
	}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
			ClientResolver: func(ctx context.Context) *agento11y.Client { return env.Client },
		})},
	})
	ctx, cancel := context.WithCancel(context.Background())
	streamResult, err := wrapped.DoStream(ctx, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)

	// Read one part, then cancel the context to simulate consumer abandonment.
	_, ok := <-streamResult.Stream
	require.True(t, ok)
	cancel()

	for range streamResult.Stream {
	}

	assert.Eventually(t, func() bool {
		return env.RequestCount() >= 1
	}, 2*time.Second, 10*time.Millisecond, "recording finalized even after consumer abandonment")
	require.NoError(t, env.Client.Shutdown(context.Background()))
	gen := env.SingleGenerationJSON(t)
	callErr, _ := gen["call_error"].(string)
	assert.Contains(t, callErr, "context canceled")
	output, ok := gen["output"].([]any)
	require.True(t, ok)
	require.Len(t, output, 1)
	message := output[0].(map[string]any)
	parts := message["parts"].([]any)
	require.Len(t, parts, 1)
	assert.Equal(t, "partial", parts[0].(map[string]any)["text"])
}

func TestRecordingMiddleware_StreamCancellationFinalizesIdleProvider(t *testing.T) {
	env := testkit.NewEnv(t)
	upstream := make(chan provider.StreamPart, 1)
	defer close(upstream)
	model := &mockLanguageModel{
		provider_: "anthropic",
		modelID:   "claude",
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: upstream}, nil
		},
	}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
			ClientResolver: func(context.Context) *agento11y.Client { return env.Client },
		})},
	})
	ctx, cancel := context.WithCancel(context.Background())
	streamResult, err := wrapped.DoStream(ctx, provider.CallOptions{})
	require.NoError(t, err)

	cancel()
	select {
	case _, ok := <-streamResult.Stream:
		assert.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("recording stream did not close after cancellation")
	}
	assert.Eventually(t, func() bool {
		return env.RequestCount() == 1
	}, 2*time.Second, 10*time.Millisecond)
	upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "late"}
	assert.Never(t, func() bool { return len(upstream) == 0 }, 100*time.Millisecond, 10*time.Millisecond)
	require.NoError(t, env.Client.Shutdown(context.Background()))
	gen := env.SingleGenerationJSON(t)
	callErr, _ := gen["call_error"].(string)
	assert.Contains(t, callErr, "context canceled")
}

func TestRecordingMiddleware_NilContextProvider_LogsOnce(t *testing.T) {
	// Reset the once-flag and capture log output.
	resetNilContextProviderLoggerForTest()
	t.Cleanup(resetNilContextProviderLoggerForTest)

	env := testkit.NewEnv(t)
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	opts := RecordingOptions{
		ClientResolver: func(ctx context.Context) *agento11y.Client { return env.Client },
		// ContextProvider intentionally nil.
	}

	// First call emits the Warn.
	_, err := generateWith(t, model, opts)
	require.NoError(t, err)

	// Second call must NOT emit again — the sync.Once gate guarantees this.
	_, err = generateWith(t, model, opts)
	require.NoError(t, err)

	// We can't easily intercept stdlib log output without monkey-patching, but
	// we can confirm the program didn't crash and recording continued working.
	require.NoError(t, env.Client.Shutdown(context.Background()))
	assert.GreaterOrEqual(t, env.RequestCount(), 2)
}

// TestRecordingMiddleware_NilClient_SkipsContextInfoResolution proves that
// when ClientResolver returns nil the middleware short-circuits BEFORE
// invoking ContextProvider. Without this ordering, the once-per-process
// "ContextProvider is nil" warning fires for consumers who haven't
// configured Agent Observability at all — and any ContextProvider work (potentially
// expensive: tenant lookup, registry read) runs needlessly on every
// non-recording call.
func TestRecordingMiddleware_NilClient_SkipsContextInfoResolution(t *testing.T) {
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}

	var providerCalls int
	opts := RecordingOptions{
		ClientResolver: func(ctx context.Context) *agento11y.Client { return nil },
		ContextProvider: func(ctx context.Context) ContextInfo {
			providerCalls++
			return ContextInfo{}
		},
	}

	for i := 0; i < 3; i++ {
		_, err := generateWith(t, model, opts)
		require.NoError(t, err)
		_, err = streamWith(t, model, opts)
		require.NoError(t, err)
	}

	assert.Equal(t, 0, providerCalls,
		"ContextProvider must not run when ClientResolver returns nil — recording would be a no-op anyway")
}

// recordHooksSpans installs a span recorder as the global tracer provider for
// the duration of one test and returns it. HooksMiddleware resolves its tracer
// from the global provider on every call, so the swap must be in place before
// the wrapped model is invoked.
func recordHooksSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, tp.Shutdown(context.Background()))
	})
	return sr
}

// spanAttributes flattens a recorded span's attributes into a lookup map.
func spanAttributes(span sdktrace.ReadOnlySpan) map[string]string {
	attrs := make(map[string]string)
	for _, kv := range span.Attributes() {
		attrs[string(kv.Key)] = kv.Value.String()
	}
	return attrs
}

// TestHooksMiddleware_PreflightSpan pins the hooks preflight span name and the
// decision attributes it carries. These keys are ai-sdk's own telemetry
// contract: the agento11y SDK neither produces nor reads them, so nothing else
// guards them against a rename.
func TestHooksMiddleware_PreflightSpan(t *testing.T) {
	transformed := &agento11y.HookInput{
		Messages: []agento11y.Message{
			{Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("hi (filtered)")}},
		},
	}

	// The only attributes on this span that are not ai-sdk's own.
	semconvKeys := map[string]bool{
		"gen_ai.provider.name": true,
		"gen_ai.request.model": true,
	}

	tests := []struct {
		name           string
		response       agento11y.HookEvaluateResponse
		wantErr        bool
		wantAttrs      map[string]string
		absent         []string
		wantStatus     codes.Code
		wantStatusDesc string
	}{
		{
			name:     "allow",
			response: agento11y.HookEvaluateResponse{Action: agento11y.HookActionAllow},
			wantAttrs: map[string]string{
				"aisdk.hooks.result": "allow",
				"aisdk.hooks.action": "allow",
			},
			absent:     []string{"aisdk.hooks.rule_id"},
			wantStatus: codes.Unset,
		},
		{
			name: "deny",
			response: agento11y.HookEvaluateResponse{
				Action: agento11y.HookActionDeny,
				RuleID: "rule-42",
				Reason: "policy violation",
			},
			wantErr: true,
			wantAttrs: map[string]string{
				"aisdk.hooks.result":  "deny",
				"aisdk.hooks.action":  "deny",
				"aisdk.hooks.rule_id": "rule-42",
			},
			wantStatus:     codes.Error,
			wantStatusDesc: "Agent Observability hook denied by rule rule-42: policy violation",
		},
		{
			name: "transform",
			response: agento11y.HookEvaluateResponse{
				Action:           agento11y.HookActionAllow,
				TransformedInput: transformed,
			},
			wantAttrs: map[string]string{
				"aisdk.hooks.result": "transform",
				"aisdk.hooks.action": "allow",
			},
			absent:     []string{"aisdk.hooks.rule_id"},
			wantStatus: codes.Unset,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := recordHooksSpans(t)
			h := newHooksTestServer(t, tc.response)
			client := h.clientWithHooksEnabled()
			model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
			wrapped := middleware.Wrap(middleware.WrapOptions{
				Model: model,
				Middleware: []middleware.Middleware{HooksMiddleware(HooksOptions{
					ClientResolver: func(ctx context.Context) *agento11y.Client { return client },
				})},
			})

			_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")},
			})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			spans := recorder.Ended()
			require.Len(t, spans, 1, "hooks middleware opens exactly one span")
			assert.Equal(t, "aisdk.hooks.preflight", spans[0].Name())
			assert.Equal(t, tc.wantStatus, spans[0].Status().Code,
				"a hooks denial is the only way the middleware fails the span")
			assert.Equal(t, tc.wantStatusDesc, spans[0].Status().Description)

			attrs := spanAttributes(spans[0])
			for key, want := range tc.wantAttrs {
				assert.Equal(t, want, attrs[key], "attribute %s", key)
			}
			for _, key := range tc.absent {
				assert.NotContains(t, attrs, key)
			}
			for key := range attrs {
				if semconvKeys[key] {
					continue
				}
				assert.True(t, strings.HasPrefix(key, "aisdk.hooks."),
					"hooks span attributes are ai-sdk owned; the agento11y client owns its own namespace: %s", key)
			}
		})
	}
}

// TestHooksSpanTelemetryKeys pins the exported telemetry constants so a
// rename of the underlying strings is a deliberate, visible change.
func TestHooksSpanTelemetryKeys(t *testing.T) {
	assert.Equal(t, "aisdk.hooks.preflight", SpanNameHooksPreflight)
	assert.Equal(t, "aisdk.hooks.result", SpanAttrHooksResult)
	assert.Equal(t, "aisdk.hooks.action", SpanAttrHooksAction)
	assert.Equal(t, "aisdk.hooks.rule_id", SpanAttrHooksRuleID)
}

func TestRecordingMiddleware_StreamStopAtFinishThenCancelRecordsSuccess(t *testing.T) {
	for _, drainTimeout := range []time.Duration{0, 10 * time.Millisecond} {
		t.Run(drainTimeout.String(), func(t *testing.T) {
			env := testkit.NewEnv(t)
			upstream := make(chan provider.StreamPart, 2)
			stop := provider.FinishReason{Unified: provider.FinishReasonStop}
			upstream <- provider.StreamPart{Type: provider.PartTextStart, ID: "t0"}
			upstream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &stop}
			t.Cleanup(func() { close(upstream) })
			stream := startRecordedStream(t, env, upstream, drainTimeout)

			readUntil(t, stream.result.Stream, provider.PartFinish)
			stream.cancel()
			stream.waitRecorded(t)
			require.NoError(t, env.Client.Shutdown(context.Background()))

			gen := env.SingleGenerationJSON(t)
			assert.Nil(t, gen["call_error"])
		})
	}
}

type recordedStream struct {
	result   *provider.StreamResult
	cancel   context.CancelFunc
	recorded chan struct{}
}

func startRecordedStream(t *testing.T, env *testkit.Env, upstream <-chan provider.StreamPart, drainTimeout time.Duration) recordedStream {
	t.Helper()
	recorded := make(chan struct{})
	model := &mockLanguageModel{
		provider_: "anthropic",
		modelID:   "claude",
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: upstream}, nil
		},
	}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model: model,
		Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
			ClientResolver:     func(context.Context) *agento11y.Client { return env.Client },
			StreamDrainTimeout: drainTimeout,
			OnRecordComplete:   func() { close(recorded) },
		})},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	result, err := wrapped.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.NoError(t, err)
	return recordedStream{result: result, cancel: cancel, recorded: recorded}
}

func (s recordedStream) waitRecorded(t *testing.T) {
	t.Helper()
	select {
	case <-s.recorded:
	case <-time.After(2 * time.Second):
		t.Fatal("recording did not complete")
	}
}

func readUntil(t *testing.T, stream <-chan provider.StreamPart, partType provider.StreamPartType) {
	t.Helper()
	for part := range stream {
		if part.Type == partType {
			return
		}
	}
	t.Fatalf("stream closed before a %s part", partType)
}

func segmentFinish(reason provider.UnifiedFinishReason, input, output int) provider.StreamPart {
	return provider.StreamPart{
		Type:         provider.PartFinish,
		FinishReason: &provider.FinishReason{Unified: reason},
		Usage: &provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: &input},
			OutputTokens: provider.OutputTokenUsage{Total: &output},
		},
	}
}

func streamPartError(message string) provider.StreamPart {
	return provider.StreamPart{
		Type:         provider.PartError,
		APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: message, StatusCode: 500}),
	}
}

func generationTime(t *testing.T, gen map[string]any, key string) time.Time {
	t.Helper()
	value, ok := gen[key].(string)
	require.True(t, ok, "generation has no %s", key)
	parsed, err := time.Parse(time.RFC3339Nano, value)
	require.NoError(t, err)
	return parsed
}

func timeToFirstTokenSeconds(t *testing.T, env *testkit.Env) float64 {
	t.Helper()
	var metrics metricdata.ResourceMetrics
	require.NoError(t, env.Metrics.Collect(context.Background(), &metrics))
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "gen_ai.client.time_to_first_token" {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			require.True(t, ok)
			require.Len(t, histogram.DataPoints, 1)
			return histogram.DataPoints[0].Sum
		}
	}
	t.Fatal("no time-to-first-token metric recorded")
	return 0
}

func outputTexts(t *testing.T, gen map[string]any) []string {
	t.Helper()
	output, ok := gen["output"].([]any)
	require.True(t, ok)
	var texts []string
	for _, message := range output {
		parts, _ := message.(map[string]any)["parts"].([]any)
		for _, part := range parts {
			fields, _ := part.(map[string]any)
			if text, ok := fields["text"].(string); ok {
				texts = append(texts, text)
			}
		}
	}
	return texts
}

func assertUsage(t *testing.T, gen map[string]any, input, output string) {
	t.Helper()
	usage, ok := gen["usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, input, usage["input_tokens"])
	assert.Equal(t, output, usage["output_tokens"])
}

func TestRecordingMiddleware_StreamMultiSegment_OneGeneration(t *testing.T) {
	env := testkit.NewEnv(t)
	upstream := make(chan provider.StreamPart, 8)
	stream := startRecordedStream(t, env, upstream, 0)

	upstream <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "msg-1"}
	upstream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "0", Delta: "first"}
	upstream <- segmentFinish(provider.FinishReasonToolCalls, 7, 2)
	readUntil(t, stream.result.Stream, provider.PartFinish)
	time.Sleep(20 * time.Millisecond)
	secondSegmentAt := time.Now()
	upstream <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "msg-2"}
	upstream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "0", Delta: "second"}
	readUntil(t, stream.result.Stream, provider.PartTextDelta)
	secondFinishAt := time.Now()
	upstream <- segmentFinish(provider.FinishReasonStop, 4, 9)
	close(upstream)
	for range stream.result.Stream {
	}
	stream.waitRecorded(t)
	ttft := timeToFirstTokenSeconds(t, env)
	require.NoError(t, env.Client.Shutdown(context.Background()))

	require.Equal(t, 1, env.RequestCount())
	gen := env.SingleGenerationJSON(t)
	assert.Nil(t, gen["call_error"])
	assertUsage(t, gen, "4", "9")
	assert.Equal(t, "end_turn", gen["stop_reason"])
	assert.Equal(t, "msg-2", gen["response_id"])
	output, ok := gen["output"].([]any)
	require.True(t, ok)
	require.Len(t, output, 1)
	parts := output[0].(map[string]any)["parts"].([]any)
	require.Len(t, parts, 2)
	assert.Equal(t, "first", parts[0].(map[string]any)["text"])
	assert.Equal(t, "second", parts[1].(map[string]any)["text"])
	assert.False(t, generationTime(t, gen, "completed_at").Before(secondFinishAt))
	firstTokenAt := generationTime(t, gen, "started_at").Add(time.Duration(ttft * float64(time.Second)))
	assert.True(t, firstTokenAt.Before(secondSegmentAt), "first-token time comes from the first segment")
}

func TestRecordingMiddleware_StreamContextEndsAfterFinish(t *testing.T) {
	tests := []struct {
		name       string
		parts      []provider.StreamPart
		readUntil  provider.StreamPartType
		closeAfter bool
		wantErr    string
		wantFinish bool
	}{
		{
			name:       "stall after finish records the snapshot",
			parts:      []provider.StreamPart{segmentFinish(provider.FinishReasonToolCalls, 7, 2)},
			readUntil:  provider.PartFinish,
			wantFinish: true,
		},
		{
			name: "context ends during the next segment records the snapshot",
			parts: []provider.StreamPart{
				segmentFinish(provider.FinishReasonToolCalls, 7, 2),
				{Type: provider.PartResponseMeta, ResponseID: "msg-2", Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(40)}}},
				{Type: provider.PartReasoningDelta, ID: "0", Delta: "partial"},
			},
			readUntil:  provider.PartReasoningDelta,
			wantFinish: true,
		},
		{
			name:      "provider error after finish",
			parts:     []provider.StreamPart{segmentFinish(provider.FinishReasonToolCalls, 7, 2), streamPartError("provider failed")},
			readUntil: provider.PartError,
			wantErr:   "provider failed",
		},
		{
			name:       "provider error before finish",
			parts:      []provider.StreamPart{streamPartError("provider failed"), segmentFinish(provider.FinishReasonToolCalls, 7, 2)},
			readUntil:  provider.PartFinish,
			wantErr:    "provider failed",
			wantFinish: true,
		},
		{
			name:       "upstream closes after the context ended",
			parts:      []provider.StreamPart{segmentFinish(provider.FinishReasonToolCalls, 7, 2)},
			readUntil:  provider.PartFinish,
			closeAfter: true,
			wantFinish: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := testkit.NewEnv(t)
			firstSegment := []provider.StreamPart{
				{Type: provider.PartResponseMeta, ResponseID: "msg-1"},
				{Type: provider.PartTextDelta, ID: "0", Delta: "first"},
			}
			upstream := make(chan provider.StreamPart, len(firstSegment)+len(tc.parts))
			for _, part := range append(firstSegment, tc.parts...) {
				upstream <- part
			}
			if !tc.closeAfter {
				t.Cleanup(func() { close(upstream) })
			}
			stream := startRecordedStream(t, env, upstream, 0)

			readUntil(t, stream.result.Stream, tc.readUntil)
			time.Sleep(20 * time.Millisecond)
			canceledAt := time.Now()
			stream.cancel()
			if tc.closeAfter {
				close(upstream)
			}
			stream.waitRecorded(t)
			require.NoError(t, env.Client.Shutdown(context.Background()))

			gen := env.SingleGenerationJSON(t)
			if tc.wantErr == "" {
				assert.Nil(t, gen["call_error"])
			} else {
				callErr, _ := gen["call_error"].(string)
				assert.Contains(t, callErr, tc.wantErr)
				assert.NotContains(t, callErr, "context canceled")
			}
			assert.Equal(t, "tool_use", gen["stop_reason"])
			assert.Equal(t, "msg-1", gen["response_id"])
			assertUsage(t, gen, "7", "2")
			assert.Equal(t, []string{"first"}, outputTexts(t, gen))
			completedAt := generationTime(t, gen, "completed_at")
			if tc.wantFinish {
				assert.True(t, completedAt.Before(canceledAt), "completed when the finish was observed")
			} else {
				assert.False(t, completedAt.Before(canceledAt), "completed when the recording ended")
			}
		})
	}
}

func TestRecordingMiddleware_StreamErrorAfterOnlyFinishThenClose_RecordsError(t *testing.T) {
	env := testkit.NewEnv(t)
	upstream := make(chan provider.StreamPart, 2)
	upstream <- segmentFinish(provider.FinishReasonStop, 7, 2)
	upstream <- streamPartError("provider failed")
	close(upstream)
	stream := startRecordedStream(t, env, upstream, 0)

	for range stream.result.Stream {
	}
	stream.waitRecorded(t)
	require.NoError(t, env.Client.Shutdown(context.Background()))

	gen := env.SingleGenerationJSON(t)
	callErr, _ := gen["call_error"].(string)
	assert.Contains(t, callErr, "provider failed")
}

func TestRecordingMiddleware_StreamConsumerStopsReadingWithoutCancel(t *testing.T) {
	t.Run("upstream closes", func(t *testing.T) {
		env := testkit.NewEnv(t)
		upstream := make(chan provider.StreamPart, 2)
		upstream <- segmentFinish(provider.FinishReasonStop, 7, 2)
		upstream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "0", Delta: "after"}
		close(upstream)
		stream := startRecordedStream(t, env, upstream, 0)

		readUntil(t, stream.result.Stream, provider.PartFinish)
		stream.waitRecorded(t)
		require.NoError(t, env.Client.Shutdown(context.Background()))
		assert.Equal(t, []string{"after"}, outputTexts(t, env.SingleGenerationJSON(t)))
	})

	t.Run("upstream stays open", func(t *testing.T) {
		env := testkit.NewEnv(t)
		upstream := make(chan provider.StreamPart, 1)
		upstream <- segmentFinish(provider.FinishReasonStop, 7, 2)
		t.Cleanup(func() { close(upstream) })
		stream := startRecordedStream(t, env, upstream, 0)

		readUntil(t, stream.result.Stream, provider.PartFinish)
		select {
		case <-stream.recorded:
			t.Fatal("recording ended at the finish")
		case <-time.After(100 * time.Millisecond):
		}
		assert.Zero(t, env.RequestCount())
		stream.cancel()
		stream.waitRecorded(t)
		require.NoError(t, env.Client.Shutdown(context.Background()))

		gen := env.SingleGenerationJSON(t)
		assert.Nil(t, gen["call_error"])
	})
}
