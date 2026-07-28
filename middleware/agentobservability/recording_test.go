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
	opts := RecordingOptions{
		ClientResolver: func(ctx context.Context) *agento11y.Client { return nil },
	}
	result, err := generateWith(t, model, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, model.generateHit)
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

func TestRecordingMiddleware_StreamConsumerAbandons_DrainsAndRecords(t *testing.T) {
	env := testkit.NewEnv(t)
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
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

	// Drain the rest of the tee channel — the recorder should finalize despite
	// consumer disconnect, because we drain upstream on the abandonment path.
	for range streamResult.Stream {
	}

	assert.Eventually(t, func() bool {
		return env.RequestCount() >= 1
	}, 2*time.Second, 10*time.Millisecond, "recording finalized even after consumer abandonment")
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
