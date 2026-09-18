package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackAcceptance_UnarySelectionAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name           string
		primaryError   error
		secondaryError error
		status         int
		outcomes       []fallback.AttemptOutcome
	}{
		{"primary", nil, nil, http.StatusOK, []fallback.AttemptOutcome{fallback.AttemptSelected}},
		{"secondary", hostileFallbackError(503), nil, http.StatusOK, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}},
		{"nonretryable", hostileFallbackError(400), nil, http.StatusFailedDependency, []fallback.AttemptOutcome{fallback.AttemptFailed}},
		{"exhausted", hostileFallbackError(503), hostileFallbackError(503), http.StatusServiceUnavailable, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptFailed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := func(err error) *observabilityTestModel {
				return &observabilityTestModel{generate: func(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
					assert.Equal(t, []provider.Message{provider.UserText("private-prompt")}, opts.Prompt)
					if err != nil {
						return nil, err
					}
					return &provider.GenerateResult{
						Content:          []provider.GenerateContentPart{{Type: provider.ContentText, Text: "public-answer", ProviderMetadata: hostileFallbackMetadata()}},
						FinishReason:     provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"},
						ProviderMetadata: hostileFallbackMetadata(),
						Request:          &provider.RequestMetadata{Body: json.RawMessage(`{"secret":"private-request-body"}`)},
						Response:         &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{Provider: "primary-instance", ModelID: "backend-primary"}, Headers: map[string]string{"Authorization": "private-credential"}, Body: json.RawMessage(`{"secret":"private-response-body"}`)},
					}, nil
				}}
			}
			h := newFallbackAcceptance(t, candidate(tc.primaryError), candidate(tc.secondaryError))
			response := httptest.NewRecorder()
			h.handler.ServeHTTP(response, h.request(context.Background(), false))
			assert.Equal(t, tc.status, response.Code)
			if tc.status == http.StatusOK {
				assert.Contains(t, response.Body.String(), "public-answer")
			}
			h.verify(t, "generate", response.Body.String(), tc.outcomes)
		})
	}
}

func TestFallbackAcceptance_StreamCommitmentAndPrivacy(t *testing.T) {
	valid := hostileFallbackParts()
	errorPart := provider.StreamPart{Type: provider.PartError, APICallError: hostileFallbackError(503)}
	for _, tc := range []struct {
		name     string
		primary  *provider.StreamResult
		err      error
		outcomes []fallback.AttemptOutcome
		types    []string
	}{
		{"setup_failure", nil, hostileFallbackError(503), []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "text-end", "finish"}},
		{"premature_eof", fallbackParts(), nil, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "text-end", "finish"}},
		{"nil_result", nil, nil, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "text-end", "finish"}},
		{"nil_channel", &provider.StreamResult{}, nil, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "text-end", "finish"}},
		{"leading_errors", fallbackParts(append([]provider.StreamPart{errorPart, errorPart}, valid...)...), nil, []fallback.AttemptOutcome{fallback.AttemptSelected}, []string{"stream-start", "error", "error", "response-metadata", "text-start", "text-delta", "text-end", "finish"}},
		{"later_error", fallbackParts(append(append([]provider.StreamPart{}, valid[:3]...), append([]provider.StreamPart{errorPart}, valid[3:]...)...)...), nil, []fallback.AttemptOutcome{fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "error", "text-end", "finish"}},
		{"postcommit_close", fallbackParts(valid[:3]...), nil, []fallback.AttemptOutcome{fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "error"}},
		{"postcommit_error_close", fallbackParts(errorPart), nil, []fallback.AttemptOutcome{fallback.AttemptSelected}, []string{"stream-start", "error", "error"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFallbackAcceptance(t,
				&observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return tc.primary, tc.err }},
				&observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return fallbackParts(valid...), nil
				}},
			)
			response := httptest.NewRecorder()
			h.handler.ServeHTTP(response, h.request(context.Background(), true))
			require.Equal(t, http.StatusOK, response.Code)
			var types []string
			for _, line := range strings.Split(response.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var event struct {
					Type    string `json:"type"`
					ModelID string `json:"modelId"`
					Delta   string `json:"delta"`
				}
				require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
				types = append(types, event.Type)
				if event.Type == "response-metadata" {
					assert.Equal(t, "public", event.ModelID)
				}
				if event.Type == "text-delta" {
					assert.Equal(t, "public-answer", event.Delta)
				}
			}
			assert.Equal(t, tc.types, types)
			h.verify(t, "stream", response.Body.String(), tc.outcomes)
		})
	}
}

func TestFallbackAcceptance_CancellationBeforeSelection(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		for _, during := range []string{"setup", "first_part", "ready_result"} {
			if mode == "generate" && during == "first_part" {
				continue
			}
			t.Run(mode+"/"+during, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				entered := make(chan struct{})
				returned := make(chan struct{})
				primary := &observabilityTestModel{
					generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
						close(entered)
						if during == "ready_result" {
							cancel()
							close(returned)
							return &provider.GenerateResult{}, nil
						}
						<-ctx.Done()
						defer close(returned)
						return nil, hostileFallbackError(503)
					},
					stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
						close(entered)
						if during == "ready_result" {
							cancel()
							close(returned)
							return fallbackParts(hostileFallbackParts()...), nil
						}
						if during == "setup" {
							<-ctx.Done()
							defer close(returned)
							return nil, hostileFallbackError(503)
						}
						parts := make(chan provider.StreamPart)
						go func() { <-ctx.Done(); close(parts); close(returned) }()
						return &provider.StreamResult{Stream: parts}, nil
					},
				}
				h := newFallbackAcceptance(t, primary, &observabilityTestModel{})
				response := httptest.NewRecorder()
				done := make(chan struct{})
				go func() { h.handler.ServeHTTP(response, h.request(ctx, mode == "stream")); close(done) }()
				awaitFallbackSignal(t, entered)
				cancel()
				awaitFallbackSignal(t, done)
				awaitFallbackSignal(t, returned)
				h.verify(t, mode, response.Body.String(), []fallback.AttemptOutcome{fallback.AttemptCanceled})
			})
		}
	}
}

func TestFallbackAcceptance_CanceledBlockedConsumer(t *testing.T) {
	for _, scenario := range []string{"silent", "continuously_ready"} {
		t.Run(scenario, func(t *testing.T) {
			parts := make(chan provider.StreamPart)
			stop := make(chan struct{})
			producerDone := make(chan struct{})
			var consumed atomic.Int64
			go func() {
				defer close(producerDone)
				defer close(parts)
				select {
				case parts <- provider.StreamPart{Type: provider.PartStreamStart}:
				case <-stop:
					return
				}
				if scenario == "silent" {
					<-stop
					return
				}
				for {
					select {
					case parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "private-output"}:
						consumed.Add(1)
					case <-stop:
						return
					}
				}
			}()
			t.Cleanup(func() { close(stop); awaitFallbackSignal(t, producerDone) })
			h := newFallbackAcceptance(t, &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: parts}, nil
			}}, &observabilityTestModel{})
			ctx, cancel := context.WithCancel(h.context(context.Background()))
			defer cancel()
			result, err := h.model.DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)
			if scenario == "continuously_ready" {
				require.Eventually(t, func() bool { return consumed.Load() >= 64 }, time.Second, time.Millisecond)
			}
			cancel()
			require.Eventually(t, func() bool {
				return countModelLogEvent(t, h.logText(), "aisdk.model.stream.cancelled") == 1
			}, time.Second, time.Millisecond, "logical cancellation must complete before downstream resumes reading")
			done := make(chan struct{})
			go func() {
				for range result.Stream {
				}
				close(done)
			}()
			awaitFallbackSignal(t, done)
			time.Sleep(150 * time.Millisecond)
			stoppedAt := consumed.Load()
			time.Sleep(30 * time.Millisecond)
			assert.Equal(t, stoppedAt, consumed.Load(), "fallback-owned draining must stop even while producer remains ready")
			h.verify(t, "stream", "", []fallback.AttemptOutcome{fallback.AttemptSelected})
		})
	}
}

type fallbackAcceptance struct {
	model   provider.LanguageModel
	handler http.Handler
	logText func() string
	calls   [2]atomic.Int32
	context func(context.Context) context.Context
	verify  func(*testing.T, string, string, []fallback.AttemptOutcome)
}

func newFallbackAcceptance(t *testing.T, primary, secondary *observabilityTestModel) *fallbackAcceptance {
	t.Helper()
	env := testkit.NewEnv(t, func(cfg *agento11y.Config) {
		cfg.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
		cfg.Hooks = agento11y.HooksConfig{Enabled: false}
	})
	var logs lockedBuffer
	var output physicalBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(&output, telemetry, 8, time.Second, time.Second)
	t.Cleanup(sink.Close)
	h := &fallbackAcceptance{}
	h.logText = logs.String
	catalog, err := buildCatalog(fallbackCatalogFile(), fallbackProviders(), http.DefaultClient, func(_ string, id string, _ ...anthropicprovider.Option) provider.LanguageModel {
		index, model := 0, primary
		if id == "backend-secondary" {
			index, model = 1, secondary
		}
		return &observabilityTestModel{
			generate: func(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
				h.calls[index].Add(1)
				if model.generate == nil {
					return nil, errors.New("unexpected secondary invocation")
				}
				return model.generate(ctx, opts)
			},
			stream: func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				h.calls[index].Add(1)
				if model.stream == nil {
					return nil, errors.New("unexpected secondary invocation")
				}
				return model.stream(ctx, opts)
			},
		}
	}, factory, sink)
	require.NoError(t, err)
	resolved, err := catalog.ResolveModel(context.Background(), "alias")
	require.NoError(t, err)
	h.model = resolved.Model
	h.handler, err = providerv4.New(providerv4.Config{Resolver: catalog, Limits: serviceTestLimits()})
	require.NoError(t, err)
	h.context = func(ctx context.Context) context.Context {
		state := &telemetryState{}
		state.observation.Store(&requestObservation{correlationID: "fallback-correlation"})
		return context.WithValue(ctx, telemetryStateKey{}, state)
	}
	h.verify = func(t *testing.T, mode, public string, outcomes []fallback.AttemptOutcome) {
		t.Helper()
		require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
		sink.Close()
		env.Shutdown(t)
		generation := env.SingleGenerationJSON(t)
		encoded, err := json.Marshal(generation)
		require.NoError(t, err)
		assert.Equal(t, "public", testkit.StringValue(t, generation, "model", "name"))
		assert.Equal(t, "fallback-correlation", testkit.StringValue(t, generation, "metadata", "gateway.correlation_id"))
		assert.Equal(t, 1, countModelLogEvent(t, logs.String(), "aisdk.model."+mode+".start"))
		terminal := 0
		for _, event := range []string{"finish", "error", "cancelled"} {
			terminal += countModelLogEvent(t, logs.String(), "aisdk.model."+mode+"."+event)
		}
		assert.Equal(t, 1, terminal)
		metrics := testMetrics(t, telemetry)
		assert.Equal(t, 1, strings.Count(metrics, "aisdk_model_requests_total{"))
		logical := string(encoded) + logs.String() + metrics
		for _, private := range []string{"private-prompt", "private-output", "public-answer", "primary-instance", "secondary-instance", "backend-primary", "backend-secondary"} {
			assert.NotContains(t, logical, private)
		}
		for _, private := range []string{"primary-instance", "secondary-instance", "backend-primary", "backend-secondary"} {
			assert.NotContains(t, public, private)
		}
		for _, private := range []string{"private-credential", "private.example", "private-header", "private-request-body", "private-response-body", "private-error", "private-data", "private-metadata"} {
			assert.NotContains(t, public+logical+output.String(), private)
		}
		lines := strings.Split(strings.TrimSpace(output.String()), "\n")
		require.Len(t, lines, len(outcomes))
		assert.Equal(t, int32(1), h.calls[0].Load())
		assert.Equal(t, int32(len(outcomes)-1), h.calls[1].Load())
		for index, line := range lines {
			var record physicalAttemptRecord
			require.NoError(t, json.Unmarshal([]byte(line), &record))
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(line), &fields))
			var keys []string
			for key := range fields {
				keys = append(keys, key)
			}
			assert.ElementsMatch(t, []string{"event", "correlation_id", "candidate_index", "provider_instance", "backend_model_id", "started_at", "decided_at", "outcome", "will_fallback", "winner"}, keys)
			assert.Equal(t, index+1, record.CandidateIndex)
			assert.Equal(t, []string{"primary-instance", "secondary-instance"}[index], record.ProviderInstance)
			assert.Equal(t, []string{"backend-primary", "backend-secondary"}[index], record.BackendModelID)
			assert.Equal(t, outcomes[index], record.Outcome)
			assert.Equal(t, index+1 < len(outcomes), record.WillFallback)
			assert.Equal(t, outcomes[index] == fallback.AttemptSelected, record.Winner)
			assert.Equal(t, "fallback-correlation", record.CorrelationID)
		}
	}
	return h
}

func (h *fallbackAcceptance) request(ctx context.Context, stream bool) *http.Request {
	request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[{"role":"user","content":[{"type":"text","text":"private-prompt"}]}]}`)).WithContext(h.context(ctx))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "private-credential")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, "alias")
	request.Header.Set(providerv4.HeaderStreaming, "false")
	if stream {
		request.Header.Set(providerv4.HeaderStreaming, "true")
	}
	return request
}

func hostileFallbackError(status int) *provider.APICallError {
	return provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: status, Message: "private-error private-credential", URL: "https://private.example", ResponseHeaders: map[string][]string{"X-Private": {"private-header"}}, ResponseBody: "private-response-body", Data: json.RawMessage(`{"private":"private-data"}`)})
}

func hostileFallbackMetadata() provider.ProviderMetadata {
	return provider.ProviderMetadata{"anthropic": json.RawMessage(`{"private":"private-metadata"}`)}
}

func hostileFallbackParts() []provider.StreamPart {
	return []provider.StreamPart{
		{Type: provider.PartResponseMeta, ModelID: "backend-primary", Provider: "primary-instance", ResponseHeaders: map[string]string{"Authorization": "private-credential"}, ProviderMetadata: hostileFallbackMetadata()},
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: "public-answer", ProviderMetadata: hostileFallbackMetadata()},
		{Type: provider.PartTextEnd, ID: "text"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"}, Usage: &provider.Usage{Raw: json.RawMessage(`{"private":"private-metadata"}`)}, ProviderMetadata: hostileFallbackMetadata()},
	}
}

func fallbackParts(parts ...provider.StreamPart) *provider.StreamResult {
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream, Request: &provider.RequestMetadata{Body: json.RawMessage(`{"private":"private-request-body"}`)}, Response: &provider.ResponseHeaders{Headers: map[string]string{"Authorization": "private-credential"}}}
}

func awaitFallbackSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("composed fallback lifecycle exceeded its cleanup bound")
	}
}
