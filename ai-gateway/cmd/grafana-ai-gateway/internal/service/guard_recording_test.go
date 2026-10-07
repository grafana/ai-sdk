package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type guardRecordingResolver struct{ model provider.LanguageModel }

func (resolver guardRecordingResolver) ResolveModel(context.Context, string) (catalog.ResolvedModel, error) {
	return catalog.ResolvedModel{ID: "canonical", Model: resolver.model}, nil
}

func guardRecordingHandler(t testing.TB, endpoint string, parts []provider.StreamPart) (http.Handler, *testkit.Env, *Telemetry) {
	t.Helper()
	env := testkit.NewEnv(t, func(c *agento11y.Config) {
		c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
		c.Hooks = agento11y.HooksConfig{Enabled: false}
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	settings := guardRuntimeSettings(endpoint)
	settings.BodyBytes, settings.RetainedBytes = 4<<20, 8<<20
	guard, err := NewGuardRuntime(settings, "development", func(string) (string, bool) { return "operator-secret", true }, telemetry)
	require.NoError(t, err)
	t.Cleanup(guard.Close)
	recorder := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
	t.Cleanup(recorder.Close)
	factory, err := NewModelObservabilityFactory(telemetry, logger, recorder, 10*time.Millisecond)
	require.NoError(t, err)
	model, err := factory("canonical", &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		ch := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			ch <- part
		}
		close(ch)
		return &provider.StreamResult{Stream: ch}, nil
	}})
	require.NoError(t, err)
	handler, err := providerv4.New(providerv4.Config{Resolver: guardRecordingResolver{model}, Guard: guard, GuardRetainedBytes: settings.RetainedBytes / 2, Limits: providerv4.Limits{RequestBytes: 1 << 20, UnaryResponseBytes: 8 << 20, StreamFrameBytes: 1 << 20, StreamParts: 100000, ModelDuration: time.Minute, StreamIdleDuration: time.Second, StreamDrainDuration: 10 * time.Millisecond}})
	require.NoError(t, err)
	return handler, env, telemetry
}

func guardRecordingRequest() *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/language-model", strings.NewReader(`{"prompt":[{"role":"user","content":[{"type":"text","text":"prompt-guard-canary"}]}]}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(providerv4.HeaderSpecificationVersion, "4")
	r.Header.Set(providerv4.HeaderStreaming, "true")
	r.Header.Set(providerv4.HeaderModelID, "alias")
	return r
}

func TestGuardedRecording_DeniedOutputPreservesUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body guardHookRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "canonical", body.Context.Model.Name)
		assert.Contains(t, body.Input.Messages[0].Parts[0].Text, "prompt-guard-canary")
		if body.Phase == guardPostflight {
			assert.Equal(t, "output-guard-canary", body.Input.Output[0].Parts[0].Text)
			_, _ = io.WriteString(w, `{"action":"deny","reason":"reason-guard-canary"}`)
		} else {
			_, _ = io.WriteString(w, `{"action":"allow"}`)
		}
	}))
	defer server.Close()
	tokens := 7
	finish := provider.FinishReason{Unified: provider.FinishReasonStop}
	handler, env, telemetry := guardRecordingHandler(t, server.URL, []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: "output-guard-canary"},
		{Type: provider.PartTextEnd, ID: "text"},
		{Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{OutputTokens: provider.OutputTokenUsage{Total: &tokens}}},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, guardRecordingRequest())
	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.NotContains(t, response.Body.String(), "data: ")
	require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
	generation := env.SingleGenerationJSON(t)
	assert.Equal(t, "7", testkit.StringValue(t, generation, "usage", "output_tokens"))
	raw, err := json.Marshal(generation)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(env.Spans.Ended()) > 0 }, time.Second, time.Millisecond)
	var spans strings.Builder
	for _, span := range env.Spans.Ended() {
		fmt.Fprint(&spans, span.Name(), span.Attributes(), span.Events(), span.Status())
	}
	for _, canary := range []string{"prompt-guard-canary", "output-guard-canary", "reason-guard-canary"} {
		assert.NotContains(t, string(raw)+testMetrics(t, telemetry)+spans.String()+response.Body.String(), canary)
	}
}

func BenchmarkGuardedStreamRecording(b *testing.B) {
	var baseline uint64
	var retained uint64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body guardHookRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Phase == guardPostflight {
			runtime.GC()
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			if memory.HeapAlloc > baseline {
				retained = max(retained, memory.HeapAlloc-baseline)
			}
		}
		_, _ = io.WriteString(w, `{"action":"allow"}`)
	}))
	defer server.Close()
	parts := []provider.StreamPart{{Type: provider.PartTextStart, ID: "text"}}
	for range 128 {
		parts = append(parts, provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: strings.Repeat("x", 1024)})
	}
	finish := provider.FinishReason{Unified: provider.FinishReasonStop}
	parts = append(parts, provider.StreamPart{Type: provider.PartTextEnd, ID: "text"}, provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{}})
	handler, _, _ := guardRecordingHandler(b, server.URL, parts)
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	baseline = memory.HeapAlloc
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, guardRecordingRequest())
		require.Equal(b, http.StatusOK, response.Code, response.Body.String())
	}
	b.ReportMetric(float64(retained), "postflight-live-B")
}
