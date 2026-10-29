package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/middleware"
	agentmiddleware "github.com/grafana/ai-sdk/middleware/agentobservability"
	"github.com/grafana/ai-sdk/provider"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBYOKObservability_LogicalExportAndCredentialPrivacy(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, status := range []int{503, 401, -1} {
			t.Run(strconv.FormatBool(streaming)+"/"+strconv.Itoa(status), func(t *testing.T) {
				env := testkit.NewEnv(t, func(c *agento11y.Config) {
					c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
					c.Hooks = agento11y.HooksConfig{Enabled: false}
				})
				var logs lockedBuffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				telemetry, err := NewTelemetry(logger)
				require.NoError(t, err)
				runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
				_, observe, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
				require.NoError(t, err)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var keys []string
				client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
					keys = append(keys, r.Header.Get("X-Api-Key"))
					if status == -1 {
						cancel()
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					raw, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Contains(t, string(raw), "application-content")
					assert.NotContains(t, string(raw), "gateway")
					assert.NotContains(t, string(raw), "dummy-")
					code := http.StatusOK
					contentType := "application/json"
					body := `{"id":"msg_1","type":"message","role":"assistant","model":"backend-private","content":[{"type":"text","text":"application-output"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
					if streaming {
						contentType = "text/event-stream"
						body = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"backend-private\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					}
					if len(keys) == 1 {
						code = status
						contentType = "application/json"
						body = `{"type":"error","error":{"type":"api_error","message":"dummy-first dummy-second"}}`
					}
					return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				})}
				wire, err := newWireHandler(execution.Config{Selector: byokSelector(client, observe), Limits: serviceTestLimits()})
				require.NoError(t, err)
				handler, headers := withAccess(t, gatewayauth.AccessModeCloud, gatewayauth.RequestBYOK, wire)
				request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[{"role":"user","content":[{"type":"text","text":"application-content"}]}],"maxOutputTokens":32,"providerOptions":{"gateway":{"byok":{"anthropic":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}}}`))
				requestID := "anthropic/native-" + strings.Repeat("x", 140)
				request = request.WithContext(ctx)
				request.Header = headers
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(providerv4.HeaderModelID, requestID)
				request.Header.Set(providerv4.HeaderSpecificationVersion, "4")
				request.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(streaming))
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				switch status {
				case -1:
					assert.Equal(t, []string{"dummy-first"}, keys)
				case 401:
					assert.Equal(t, http.StatusFailedDependency, response.Code)
					assert.Equal(t, []string{"dummy-first"}, keys)
				default:
					assert.Equal(t, http.StatusOK, response.Code)
					assert.Equal(t, []string{"dummy-first", "dummy-second"}, keys)
				}
				runtime.Close(context.Background())
				generation := env.SingleGenerationJSON(t)
				assert.Equal(t, "anthropic", testkit.StringValue(t, generation, "model", "provider"))
				assert.Equal(t, requestID, testkit.StringValue(t, generation, "model", "name"))
				exported, err := json.Marshal(generation)
				require.NoError(t, err)
				failed := execution.Attempt{Provider: "anthropic", ModelID: strings.TrimPrefix(requestID, "anthropic/"), Outcome: fallback.AttemptFailed, Error: &execution.Failure{Message: "dummy-first dummy-second", Type: "api_error", StatusCode: status}}
				switch status {
				case -1:
					assert.Empty(t, responseOverviews(t, response.Body.String()))
				case 401:
					assert.Equal(t, []execution.Overview{{RequestedModelID: requestID, Attempts: []execution.Attempt{failed}}}, responseOverviews(t, response.Body.String()))
				default:
					selected := execution.Attempt{Provider: failed.Provider, ModelID: failed.ModelID, Outcome: fallback.AttemptSelected}
					assert.Equal(t, []execution.Overview{{RequestedModelID: requestID, Attempts: []execution.Attempt{failed, selected}}}, responseOverviews(t, response.Body.String()))
				}
				assert.NotContains(t, response.Body.String(), `"nativeError"`)
				metrics := testMetrics(t, telemetry)
				assert.Contains(t, metrics, `model="byok"`)
				assert.NotContains(t, metrics, requestID)
				assert.Contains(t, logs.String(), requestID)
				for _, secret := range []string{"dummy-first", "dummy-second"} {
					assert.NotContains(t, logs.String()+metrics+string(exported), secret)
				}
				for _, private := range []string{"backend-private", "application-content", "application-output"} {
					assert.NotContains(t, logs.String()+metrics+string(exported), private)
				}
			})
		}
	}
}

func TestNewAgentObservabilityRuntime_DisabledIsNoOp(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.NoError(t, err)
	runtime, err := NewAgentObservabilityRuntime(config.AgentObservabilitySettings{}, "", telemetry)
	require.NoError(t, err)
	require.NotNil(t, runtime)
	assert.Nil(t, runtime.client)
	runtime.Close(context.Background())
}

func TestAgentObservabilityRuntime_CloseWaitsForAcquiredRecordings(t *testing.T) {
	env := testkit.NewEnv(t)
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{
		client:    env.Client,
		telemetry: telemetry,
	}
	require.Same(t, env.Client, runtime.acquireClient(context.Background()))
	require.Same(t, env.Client, runtime.acquireClient(context.Background()))
	closed := make(chan struct{})
	go func() {
		runtime.Close(context.Background())
		close(closed)
	}()
	require.Eventually(t, func() bool {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return runtime.closing
	}, time.Second, time.Millisecond)
	assert.Nil(t, runtime.acquireClient(context.Background()))
	select {
	case <-closed:
		t.Fatal("runtime closed before the acquired recording completed")
	case <-time.After(20 * time.Millisecond):
	}
	runtime.recordingComplete()
	select {
	case <-closed:
		t.Fatal("runtime closed before every acquired recording completed")
	case <-time.After(20 * time.Millisecond):
	}
	runtime.recordingComplete()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("runtime did not close after the acquired recording completed")
	}
}

func TestAgentObservabilityRuntime_CloseEndsAtTheSharedDeadline(t *testing.T) {
	env := testkit.NewEnv(t)
	var logs lockedBuffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{
		client:    env.Client,
		telemetry: telemetry,
	}
	require.Same(t, env.Client, runtime.acquireClient(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	runtime.Close(ctx)
	assert.Less(t, time.Since(started), time.Second)
	assert.Nil(t, runtime.acquireClient(context.Background()))
	assert.Contains(t, testMetrics(t, telemetry), `grafana_ai_gateway_agento11y_export_failures_total{class="shutdown"}`)
	runtime.recordingComplete()
}

func TestAgentObservabilityRuntime_ConcurrentAcquireAndClose(t *testing.T) {
	env := testkit.NewEnv(t)
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{
		client:    env.Client,
		telemetry: telemetry,
	}
	start := make(chan struct{})
	var callers sync.WaitGroup
	for range 64 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			<-start
			if runtime.acquireClient(context.Background()) != nil {
				runtime.recordingComplete()
			}
		}()
	}
	closed := make(chan struct{})
	go func() {
		<-start
		runtime.Close(context.Background())
		close(closed)
	}()
	close(start)
	callers.Wait()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent runtime close did not finish")
	}
	assert.Nil(t, runtime.acquireClient(context.Background()))
}

func TestNewModelObservabilityFactory_CloseWaitsForAbandonedStreamRecording(t *testing.T) {
	env := testkit.NewEnv(t, func(configuration *agento11y.Config) {
		configuration.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
		configuration.Hooks = agento11y.HooksConfig{Enabled: false}
	})
	var logs lockedBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{
		client:    env.Client,
		telemetry: telemetry,
	}
	factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)
	upstream := make(chan provider.StreamPart)
	model, err := factory("grafana/assistant", &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: upstream}, nil
	}})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	result, err := model.DoStream(ctx, provider.CallOptions{})
	require.NoError(t, err)
	cancel()
	closed := make(chan struct{})
	go func() {
		runtime.Close(context.Background())
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not wait for and export the abandoned stream recording")
	}
	select {
	case _, ok := <-result.Stream:
		assert.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("abandoned stream output did not close")
	}
	assert.Equal(t, 1, env.RequestCount())
	assert.NotContains(t, logs.String(), `"msg":"agent observability export failed"`)
	close(upstream)
}

func TestNewAgentObservabilityRuntime_HTTPExportUsesOneResolvedSecretAndSafePayload(t *testing.T) {
	type capturedRequest struct {
		authorization string
		path          string
		body          string
	}
	captured := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var export map[string]any
		if err := json.Unmarshal(body, &export); err != nil {
			http.Error(w, "decode body", http.StatusBadRequest)
			return
		}
		generations, ok := export["generations"].([]any)
		if !ok || len(generations) != 1 {
			http.Error(w, "generation count", http.StatusBadRequest)
			return
		}
		generation, ok := generations[0].(map[string]any)
		generationID, idOK := generation["id"].(string)
		if !ok || !idOK || generationID == "" {
			http.Error(w, "generation id", http.StatusBadRequest)
			return
		}
		captured <- capturedRequest{
			authorization: request.Header.Get("Authorization"),
			path:          request.URL.Path,
			body:          string(body),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"results":[{"generation_id":"`+generationID+`","accepted":true}]}`)
	}))
	defer server.Close()

	var logs lockedBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime, err := newAgentObservabilityRuntime(config.AgentObservabilitySettings{
		Enabled:  true,
		Protocol: config.AgentObservabilityHTTP,
		Endpoint: server.URL,
	}, "agent-secret", telemetry, agentExportTuning{
		queueSize:       8,
		batchSize:       1,
		payloadMaxBytes: 1 << 20,
		maxRetries:      1,
		initialBackoff:  time.Millisecond,
		maxBackoff:      time.Millisecond,
		flushInterval:   time.Hour,
	})
	require.NoError(t, err)
	factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)
	model, err := factory("grafana/assistant", &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{
			Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "output-private"}},
			FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "finish-private"},
			Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
				ID: "response-private", Provider: "anthropic", ModelID: "backend-private",
			}},
		}, nil
	}})
	require.NoError(t, err)
	_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("prompt-private")}})
	require.NoError(t, err)
	runtime.Close(context.Background())
	request := <-captured
	assert.Equal(t, "Bearer agent-secret", request.authorization)
	assert.Equal(t, "/api/v1/generations:export", request.path)
	for _, private := range []string{"prompt-private", "output-private", "finish-private", "response-private", "anthropic", "backend-private", "agent-secret"} {
		assert.NotContains(t, request.body, private)
		assert.NotContains(t, logs.String(), private)
	}
	assert.Contains(t, request.body, "grafana/assistant")
	assert.Contains(t, request.body, `"stop_reason":"stop"`)
}

func TestNewAgentObservabilityRuntime_RejectedHTTPBatchIsOneFailOpenFailure(t *testing.T) {
	private := "rejection-private"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(w, "read", http.StatusBadRequest)
			return
		}
		var export struct {
			Generations []struct {
				ID string `json:"id"`
			} `json:"generations"`
		}
		if err := json.Unmarshal(body, &export); err != nil || len(export.Generations) != 1 {
			http.Error(w, "decode", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"results":[{"generation_id":"`+export.Generations[0].ID+`","accepted":false,"error":"`+private+`"}]}`)
	}))
	defer server.Close()

	var logs lockedBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime, err := newAgentObservabilityRuntime(config.AgentObservabilitySettings{
		Enabled:  true,
		Protocol: config.AgentObservabilityHTTP,
		Endpoint: server.URL,
	}, "agent-secret", telemetry, agentExportTuning{
		queueSize:       4,
		batchSize:       1,
		payloadMaxBytes: 1 << 20,
		maxRetries:      1,
		initialBackoff:  time.Millisecond,
		maxBackoff:      time.Millisecond,
		flushInterval:   time.Hour,
	})
	require.NoError(t, err)
	defer runtime.Close(context.Background())
	factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)
	expected := &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "model-private"}}}
	model, err := factory("grafana/assistant", &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return expected, nil
	}})
	require.NoError(t, err)
	result, err := model.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assert.Same(t, expected, result)
	require.Eventually(t, func() bool {
		return strings.Contains(testMetrics(t, telemetry), `grafana_ai_gateway_agento11y_export_failures_total{class="rejected"} 1`)
	}, time.Second, 10*time.Millisecond)
	metrics := testMetrics(t, telemetry)
	assert.NotContains(t, metrics, `class="transport"`)
	assert.Equal(t, 1, strings.Count(logs.String(), "agent observability export failed"))
	for _, value := range []string{private, "agent-secret", "model-private", server.URL} {
		assert.NotContains(t, metrics, value)
		assert.NotContains(t, logs.String(), value)
	}
}

func TestAgentDiagnosticWriter_EmitsOnlyFixedClasses(t *testing.T) {
	var logs bytes.Buffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	private := "https://secret.example bearer-private payload-private"
	writer := agentDiagnosticWriter{telemetry: telemetry}
	for _, message := range []string{
		"agento11y generation export failed: http generation export failed: " + private,
		"agento11y generation export failed: generation export rejected: " + private,
		"unexpected failure: " + private,
		"unexpected failure: " + strings.Repeat("x", maxAgentDiagnosticBytes+1) + private,
		"agento11y generation export response requested=1 results=1",
	} {
		written, err := writer.Write([]byte(message))
		require.NoError(t, err)
		assert.Equal(t, len(message), written)
	}
	serialized := logs.String()
	assert.Contains(t, serialized, `"class":"transport"`)
	assert.Contains(t, serialized, `"class":"rejected"`)
	assert.Contains(t, serialized, `"class":"unknown"`)
	assert.Equal(t, 4, strings.Count(serialized, "agent observability export failed"))
	assert.NotContains(t, serialized, private)
}

func TestAgentDiagnosticWriter_CountsRejectedBatchOnce(t *testing.T) {
	var logs bytes.Buffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	writer := agentDiagnosticWriter{telemetry: telemetry}
	private := "generation-private error-private"
	for _, message := range []string{
		"agento11y generation rejected id=" + private,
		"AGENTO11Y generation export failed: generation export rejected: " + private,
	} {
		written, err := writer.Write([]byte(message))
		require.NoError(t, err)
		assert.Equal(t, len(message), written)
	}
	metrics := testMetrics(t, telemetry)
	assert.Contains(t, metrics, `grafana_ai_gateway_agento11y_export_failures_total{class="rejected"} 1`)
	assert.NotContains(t, metrics, `class="transport"`)
	assert.Equal(t, 1, strings.Count(logs.String(), "agent observability export failed"))
	assert.NotContains(t, logs.String(), private)
}

func TestAgentDiagnosticWriter_UntrustedSuffixCannotChooseFailureClass(t *testing.T) {
	var logs bytes.Buffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	writer := agentDiagnosticWriter{telemetry: telemetry}
	private := "upstream says shutdown failed and generation export rejected: private"
	written, err := writer.Write([]byte("agento11y generation export failed: http generation export failed: " + private))
	require.NoError(t, err)
	assert.Positive(t, written)
	metrics := testMetrics(t, telemetry)
	assert.Contains(t, metrics, `grafana_ai_gateway_agento11y_export_failures_total{class="transport"} 1`)
	assert.NotContains(t, metrics, `class="shutdown"`)
	assert.NotContains(t, metrics, `class="rejected"`)
	assert.NotContains(t, logs.String(), private)
}

func TestAgentRecordErrors_AreBoundedAndNeverLeakRawDetails(t *testing.T) {
	var logs bytes.Buffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{telemetry: telemetry}
	for _, recordErr := range []error{
		agento11y.ErrQueueFull,
		agento11y.ErrValidationFailed,
		agento11y.ErrClientShutdown,
		errors.New("https://private.example bearer-private payload-private"),
	} {
		runtime.recordError(recordErr)
	}
	metrics := testMetrics(t, telemetry)
	for _, class := range []string{"queue_full", "serialization", "shutdown", "unknown"} {
		assert.Contains(t, metrics, `class="`+class+`"`)
		assert.Contains(t, logs.String(), `"class":"`+class+`"`)
	}
	for _, private := range []string{"https://private.example", "bearer-private", "payload-private"} {
		assert.NotContains(t, metrics, private)
		assert.NotContains(t, logs.String(), private)
	}
}

func TestFilterAgentGeneration_ClosedMetadataAndIdentityPolicy(t *testing.T) {
	generation := agento11y.Generation{
		Model:               agento11y.ModelRef{Provider: "grafana", Name: "grafana/assistant"},
		ResponseID:          "response-private",
		ResponseModel:       "backend-private",
		UserID:              "user-private",
		AgentName:           "agent-private",
		AgentVersion:        "version-private",
		ParentGenerationIDs: []string{"parent-private"},
		Tags:                map[string]string{"private": "tag-private"},
		Metadata: map[string]any{
			"gateway.correlation_id": "correlation",
			"gateway.namespace":      "verified-namespace",
			"provider.private":       "provider-option-private",
			"gateway.caller_service": strings.Repeat("x", maxObservationValueLen+1),
		},
		SystemPrompt: "content preserved for metadata-only stripping",
		Usage:        agento11y.TokenUsage{InputTokens: 2, OutputTokens: 3},
		CallError:    "provider-error-private",
	}
	filtered := filterAgentGeneration(agentmiddleware.GenerationFilterInput{
		Generation:   generation,
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "raw-private"},
	})

	assert.Equal(t, generation.Model, filtered.Model)
	assert.Equal(t, agento11y.TokenUsage{InputSemantics: agento11y.TokenInputSemanticsInclusive, InputTokens: 2, OutputTokens: 3}, filtered.Usage)
	assert.Equal(t, generation.SystemPrompt, filtered.SystemPrompt)
	assert.Equal(t, map[string]any{
		"gateway.correlation_id": "correlation",
		"gateway.namespace":      "verified-namespace",
	}, filtered.Metadata)
	assert.Empty(t, filtered.ResponseID)
	assert.Empty(t, filtered.ResponseModel)
	assert.Empty(t, filtered.UserID)
	assert.Empty(t, filtered.AgentName)
	assert.Empty(t, filtered.AgentVersion)
	assert.Empty(t, filtered.ParentGenerationIDs)
	assert.Empty(t, filtered.Tags)
	assert.Empty(t, filtered.CallError)
	assert.Equal(t, "stop", filtered.StopReason)
}

func TestNewModelObservabilityFactory_SharedRegistryAndPassThrough(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	factory, _, err := NewModelObservabilityFactories(telemetry, logger, nil, 10*time.Millisecond)
	require.NoError(t, err)
	_, _, err = NewModelObservabilityFactories(telemetry, logger, nil, 10*time.Millisecond)
	require.Error(t, err, "duplicate model collector registration must fail")

	result := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "output-private"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "raw-private"},
		Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
			ID: "response-private", Provider: "anthropic", ModelID: "backend-private",
		}},
	}
	var gotOptions provider.CallOptions
	lower := &observabilityTestModel{generate: func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		gotOptions = options
		return result, nil
	}}
	model, err := factory("grafana/assistant", lower)
	require.NoError(t, err)
	state := &telemetryState{}
	state.observation.Store(&requestObservation{correlationID: "correlation", callerService: "caller"})
	ctx := context.WithValue(context.Background(), telemetryStateKey{}, state)
	options := provider.CallOptions{
		Prompt:  []provider.Message{provider.UserText("prompt-private")},
		Headers: map[string]string{"Authorization": "bearer-private"},
		ProviderOptions: provider.ProviderOptions{
			"private": provider.RawProviderOption{Key: "private", Raw: json.RawMessage(`{"url":"https://private.example"}`)},
		},
	}
	got, err := model.DoGenerate(ctx, options)
	require.NoError(t, err)
	assert.Same(t, result, got)
	assert.Equal(t, options, gotOptions)
	assert.Equal(t, "grafana", model.Provider())
	assert.Equal(t, "grafana/assistant", model.ModelID())

	metrics := testMetrics(t, telemetry)
	assert.Equal(t, 1, strings.Count(metrics, "aisdk_model_requests_total{"))
	assert.Contains(t, metrics, `provider="grafana"`)
	assert.Contains(t, metrics, `model="grafana/assistant"`)
	for _, private := range []string{"prompt-private", "output-private", "raw-private", "response-private", "anthropic", "backend-private", "bearer-private", "https://private.example"} {
		assert.NotContains(t, logs.String(), private)
		assert.NotContains(t, metrics, private)
	}
	assert.Contains(t, logs.String(), "correlation")
	assert.Contains(t, logs.String(), "caller")
}

func TestModelObservationChain_ExactRequestAndResponseOrder(t *testing.T) {
	var order []string
	marker := func(name string) middleware.Middleware {
		return middleware.Middleware{WrapGenerate: func(ctx context.Context, params middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
			assert.Equal(t, "grafana", params.Model.Provider())
			assert.Equal(t, "grafana/assistant", params.Model.ModelID())
			order = append(order, name+":request")
			result, err := params.DoGenerate(ctx)
			order = append(order, name+":response")
			return result, err
		}}
	}
	agent := marker("agent")
	chain := modelObservationChain{
		contextBridge:    marker("context"),
		agentRecording:   &agent,
		structuredLogger: marker("logger"),
		prometheus:       marker("prometheus"),
	}
	lower := &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		order = append(order, "provider")
		return &provider.GenerateResult{}, nil
	}}
	model, err := chain.wrap("grafana/assistant", lower)
	require.NoError(t, err)
	_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"context:request", "agent:request", "logger:request", "prometheus:request", "provider",
		"prometheus:response", "logger:response", "agent:response", "context:response",
	}, order)
}

func TestModelObservationChain_WP9LowerModelSeamStaysBelowOneLogicalWrapper(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	factory, _, err := NewModelObservabilityFactories(telemetry, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, 10*time.Millisecond)
	require.NoError(t, err)

	var lowerCalls atomic.Int64
	lowerResult := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "unchanged"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
			Provider: "physical-provider", ModelID: "physical-candidate",
		}},
	}
	placeholderFallback := &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		lowerCalls.Add(1)
		return lowerResult, nil
	}}
	logical, err := factory("grafana/assistant", placeholderFallback)
	require.NoError(t, err)

	got, err := logical.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assert.Same(t, lowerResult, got)
	assert.Equal(t, int64(1), lowerCalls.Load())
	assert.Equal(t, "grafana", logical.Provider())
	assert.Equal(t, "grafana/assistant", logical.ModelID())
	assert.Equal(t, 1, strings.Count(testMetrics(t, telemetry), "aisdk_model_requests_total{"))
}

func TestNewModelObservabilityFactory_AgentExportIsCanonicalMetadataOnly(t *testing.T) {
	env := testkit.NewEnv(t, func(configuration *agento11y.Config) {
		configuration.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
		configuration.Hooks = agento11y.HooksConfig{Enabled: false}
	})
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
	factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)

	inputTokens, outputTokens := 60, 3
	noCache, cacheRead, cacheWrite := 10, 30, 20
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "output-private"}},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: &inputTokens, NoCache: &noCache, CacheRead: &cacheRead, CacheWrite: &cacheWrite},
			OutputTokens: provider.OutputTokenUsage{Total: &outputTokens},
			Raw:          json.RawMessage(`{"server_tool_use":{"web_search_requests":7}}`),
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "finish-private"},
		ProviderMetadata: provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"private":"metadata-private"}`),
		},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ID: "response-private", Provider: "anthropic", ModelID: "backend-private"},
			Headers:          map[string]string{"X-Private": "header-private"},
			Body:             json.RawMessage(`{"private":"body-private"}`),
		},
	}
	lower := &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }}
	model, err := factory("grafana/assistant", lower)
	require.NoError(t, err)
	state := &telemetryState{}
	state.observation.Store(&requestObservation{correlationID: "correlation", callerService: "caller", namespace: "namespace"})
	ctx := context.WithValue(context.Background(), telemetryStateKey{}, state)
	maxTokens := 64
	got, err := model.DoGenerate(ctx, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("prompt-private")},
		MaxOutputTokens: &maxTokens,
		Headers:         map[string]string{"Authorization": "bearer-private"},
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":2048},"topology":"topology-private"}`)},
		},
	})
	require.NoError(t, err)
	assert.Same(t, result, got)
	env.Shutdown(t)

	generation := env.SingleGenerationJSON(t)
	serialized, err := json.Marshal(generation)
	require.NoError(t, err)
	export := string(serialized)
	for _, private := range []string{
		"prompt-private", "output-private", "finish-private", "metadata-private", "response-private",
		"anthropic", "backend-private", "header-private", "body-private", "bearer-private", "budgetTokens", "web_search_requests", "topology-private",
	} {
		assert.NotContains(t, export, private)
	}
	assert.Equal(t, "grafana", testkit.StringValue(t, generation, "model", "provider"))
	assert.Equal(t, "grafana/assistant", testkit.StringValue(t, generation, "model", "name"))
	metadata, ok := generation["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "correlation", metadata["gateway.correlation_id"])
	assert.Equal(t, "caller", metadata["gateway.caller_service"])
	assert.Equal(t, "namespace", metadata["gateway.namespace"])
	assert.Equal(t, "stop", generation["stop_reason"])
	assert.Equal(t, "TOKEN_INPUT_SEMANTICS_INCLUSIVE", testkit.StringValue(t, generation, "usage", "input_semantics"))
	assert.Equal(t, "60", testkit.StringValue(t, generation, "usage", "input_tokens"))
	assert.Equal(t, "3", testkit.StringValue(t, generation, "usage", "output_tokens"))
	assert.Equal(t, "63", testkit.StringValue(t, generation, "usage", "total_tokens"))
	assert.Equal(t, "30", testkit.StringValue(t, generation, "usage", "cache_read_input_tokens"))
	assert.Equal(t, "20", testkit.StringValue(t, generation, "usage", "cache_write_input_tokens"))
	assert.NotContains(t, logs.String()+testMetrics(t, telemetry), "web_search_requests")
}

func TestNewModelObservabilityFactory_UnaryProviderFailureIsSafeAndFailThrough(t *testing.T) {
	env := testkit.NewEnv(t, func(configuration *agento11y.Config) {
		configuration.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
		configuration.Hooks = agento11y.HooksConfig{Enabled: false}
	})
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
	factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)
	providerError := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:      "error-private bearer-private",
		URL:          "https://provider.private/path",
		StatusCode:   503,
		ResponseBody: "body-private",
	})
	model, err := factory("grafana/assistant", &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, providerError
	}})
	require.NoError(t, err)
	state := &telemetryState{}
	state.observation.Store(&requestObservation{correlationID: "failure-correlation", callerService: "caller"})
	ctx := context.WithValue(context.Background(), telemetryStateKey{}, state)
	result, gotErr := model.DoGenerate(ctx, provider.CallOptions{
		Prompt:  []provider.Message{provider.UserText("prompt-private")},
		Headers: map[string]string{"Authorization": "request-secret"},
	})
	assert.Nil(t, result)
	assert.ErrorIs(t, gotErr, providerError)
	require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
	env.Shutdown(t)

	assert.Equal(t, 1, countModelLogEvent(t, logs.String(), "aisdk.model.generate.start"))
	assert.Equal(t, 1, countModelLogEvent(t, logs.String(), "aisdk.model.generate.error"))
	metrics := testMetrics(t, telemetry)
	generation := env.SingleGenerationJSON(t)
	encoded, err := json.Marshal(generation)
	require.NoError(t, err)
	allTelemetry := logs.String() + metrics + string(encoded)
	for _, private := range []string{
		"error-private", "bearer-private", "provider.private", "body-private", "prompt-private", "request-secret",
	} {
		assert.NotContains(t, allTelemetry, private)
	}
	assert.Equal(t, "grafana", testkit.StringValue(t, generation, "model", "provider"))
	assert.Equal(t, "grafana/assistant", testkit.StringValue(t, generation, "model", "name"))
	callError := testkit.StringValue(t, generation, "call_error")
	assert.NotEmpty(t, callError)
	assert.NotContains(t, callError, "private")
}

func TestNewModelObservabilityFactory_RecorderFailureDoesNotFailModelCall(t *testing.T) {
	env := testkit.NewEnv(t, func(configuration *agento11y.Config) {
		configuration.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
	})
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
	factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)
	expected := &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
	model, err := factory("grafana/assistant", &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return expected, nil
	}})
	require.NoError(t, err)
	require.NoError(t, env.Client.Shutdown(context.Background()))

	got, err := model.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assert.Same(t, expected, got)
	metrics := testMetrics(t, telemetry)
	assert.Contains(t, metrics, `grafana_ai_gateway_agento11y_export_failures_total{class="shutdown"} 1`)
	assert.Contains(t, logs.String(), `"class":"shutdown"`)
}

func TestNewModelObservabilityFactory_StreamIsObservedOnceAndPassesThrough(t *testing.T) {
	env := testkit.NewEnv(t, func(configuration *agento11y.Config) {
		configuration.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
		configuration.Hooks = agento11y.HooksConfig{Enabled: false}
	})
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
	factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)

	inputTokens, outputTokens := 60, 6
	noCache, cacheRead, cacheWrite := 10, 30, 20
	finish := provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "finish-private"}
	privateError := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:      "error-private",
		URL:          "https://provider.private/path",
		StatusCode:   503,
		ResponseBody: "body-private",
	})
	parts := []provider.StreamPart{
		{Type: provider.PartResponseMeta, ResponseID: "response-private", Provider: "anthropic", ModelID: "backend-private", ResponseHeaders: map[string]string{"X-Private": "header-private"}, ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"topology":"topology-private"}`)}},
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: "output-private", Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &inputTokens, NoCache: &noCache, CacheRead: &cacheRead, CacheWrite: &cacheWrite}}},
		{Type: provider.PartError, APICallError: privateError},
		{Type: provider.PartTextDelta, ID: "text", Delta: "later-private"},
		{Type: provider.PartTextEnd, ID: "text"},
		{Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{OutputTokens: provider.OutputTokenUsage{Total: &outputTokens}}},
	}
	requestMetadata := &provider.RequestMetadata{Body: json.RawMessage(`{"private":"request-private"}`)}
	responseHeaders := &provider.ResponseHeaders{Headers: map[string]string{"X-Private": "response-header-private"}}
	lower := &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			stream <- part
		}
		close(stream)
		return &provider.StreamResult{Stream: stream, Request: requestMetadata, Response: responseHeaders}, nil
	}}
	model, err := factory("grafana/assistant", lower)
	require.NoError(t, err)
	state := &telemetryState{}
	state.observation.Store(&requestObservation{correlationID: "stream-correlation", callerService: "caller"})
	ctx := context.WithValue(context.Background(), telemetryStateKey{}, state)
	result, err := model.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("prompt-private")}})
	require.NoError(t, err)
	assert.Same(t, requestMetadata, result.Request)
	assert.Same(t, responseHeaders, result.Response)
	var received []provider.StreamPart
	for part := range result.Stream {
		received = append(received, part)
	}
	assert.Equal(t, parts, received)
	require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
	env.Shutdown(t)

	assert.Equal(t, 1, countModelLogEvent(t, logs.String(), "aisdk.model.stream.start"))
	assert.Equal(t, 1, countModelLogEvent(t, logs.String(), "aisdk.model.stream.error"))
	assert.NotContains(t, logs.String(), "aisdk.model.stream.part")
	metrics := testMetrics(t, telemetry)
	assert.Equal(t, 1, strings.Count(metrics, "aisdk_model_requests_total{"))
	assert.Contains(t, metrics, `provider="grafana"`)
	assert.Contains(t, metrics, `model="grafana/assistant"`)
	generation := env.SingleGenerationJSON(t)
	encoded, err := json.Marshal(generation)
	require.NoError(t, err)
	allTelemetry := logs.String() + metrics + string(encoded)
	for _, private := range []string{
		"prompt-private", "output-private", "later-private", "finish-private", "error-private",
		"provider.private", "body-private", "response-private", "anthropic", "backend-private",
		"header-private", "request-private", "response-header-private", "topology-private",
	} {
		assert.NotContains(t, allTelemetry, private)
	}
	assert.Equal(t, "grafana", testkit.StringValue(t, generation, "model", "provider"))
	assert.Equal(t, "grafana/assistant", testkit.StringValue(t, generation, "model", "name"))
	assert.Equal(t, "stop", generation["stop_reason"])
	assert.Equal(t, "TOKEN_INPUT_SEMANTICS_INCLUSIVE", testkit.StringValue(t, generation, "usage", "input_semantics"))
	assert.Equal(t, "60", testkit.StringValue(t, generation, "usage", "input_tokens"))
	assert.Equal(t, "6", testkit.StringValue(t, generation, "usage", "output_tokens"))
	assert.Equal(t, "66", testkit.StringValue(t, generation, "usage", "total_tokens"))
	assert.Equal(t, "30", testkit.StringValue(t, generation, "usage", "cache_read_input_tokens"))
	assert.Equal(t, "20", testkit.StringValue(t, generation, "usage", "cache_write_input_tokens"))
}

func TestNewModelObservabilityFactory_CanceledStreamsCloseWithinBoundedDrain(t *testing.T) {
	for _, scenario := range []string{"silent_cancel", "continuously_ready_timeout"} {
		t.Run(scenario, func(t *testing.T) {
			env := testkit.NewEnv(t, func(configuration *agento11y.Config) {
				configuration.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
				configuration.Hooks = agento11y.HooksConfig{Enabled: false}
			})
			var logs lockedBuffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			telemetry, err := NewTelemetry(logger)
			require.NoError(t, err)
			runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
			const drainTimeout = 10 * time.Millisecond
			factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, drainTimeout)
			require.NoError(t, err)

			stream := make(chan provider.StreamPart)
			stopProducer := make(chan struct{})
			producerDone := make(chan struct{})
			var produced atomic.Int64
			if scenario == "continuously_ready_timeout" {
				go func() {
					defer close(producerDone)
					defer close(stream)
					for {
						select {
						case stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "private-output"}:
							produced.Add(1)
						case <-stopProducer:
							return
						}
					}
				}()
			}
			model, err := factory("grafana/assistant", &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: stream}, nil
			}})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			if scenario == "continuously_ready_timeout" {
				ctx, cancel = context.WithTimeout(context.Background(), 5*time.Millisecond)
			}
			defer cancel()
			result, err := model.DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)
			terminalEvent := "aisdk.model.stream.cancelled"
			if scenario == "continuously_ready_timeout" {
				<-ctx.Done()
				terminalEvent = "aisdk.model.stream.error"
			} else {
				cancel()
			}
			closed := make(chan struct{})
			go func() {
				for range result.Stream {
				}
				close(closed)
			}()
			select {
			case <-closed:
			case <-time.After(10 * drainTimeout):
				t.Fatal("composed output stream did not close within bounded cleanup")
			}
			require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
			if scenario == "continuously_ready_timeout" {
				time.Sleep(4 * drainTimeout)
				stoppedAt := produced.Load()
				time.Sleep(2 * drainTimeout)
				assert.Equal(t, stoppedAt, produced.Load(), "observer drains must stop receiving after their absolute deadlines")
				close(stopProducer)
				<-producerDone
			}
			env.Shutdown(t)
			assert.Equal(t, 1, countModelLogEvent(t, logs.String(), terminalEvent))
			metrics := testMetrics(t, telemetry)
			assert.Equal(t, 1, strings.Count(metrics, "aisdk_model_requests_total{"))
			assert.NotContains(t, logs.String()+metrics, "private-output")
		})
	}
}

func TestNewModelObservabilityFactory_NormalAndPrematureStreamsFinalizeOnce(t *testing.T) {
	for _, scenario := range []string{"normal_strongest_usage", "premature_close"} {
		t.Run(scenario, func(t *testing.T) {
			env := testkit.NewEnv(t, func(configuration *agento11y.Config) {
				configuration.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
				configuration.Hooks = agento11y.HooksConfig{Enabled: false}
			})
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			telemetry, err := NewTelemetry(logger)
			require.NoError(t, err)
			runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
			factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
			require.NoError(t, err)

			inputLow, inputHigh, outputLow, outputHigh := 2, 5, 1, 3
			finish := provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "finish-private"}
			parts := []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "text"},
				{Type: provider.PartTextDelta, ID: "text", Delta: "output-private", Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &inputLow}, OutputTokens: provider.OutputTokenUsage{Total: &outputLow}}},
			}
			if scenario == "normal_strongest_usage" {
				parts = append(parts,
					provider.StreamPart{Type: provider.PartTextEnd, ID: "text", Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &inputHigh}}},
					provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{OutputTokens: provider.OutputTokenUsage{Total: &outputHigh}}},
				)
			}
			model, err := factory("grafana/assistant", &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				stream := make(chan provider.StreamPart, len(parts))
				for _, part := range parts {
					stream <- part
				}
				close(stream)
				return &provider.StreamResult{Stream: stream}, nil
			}})
			require.NoError(t, err)
			result, err := model.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			var received []provider.StreamPart
			for part := range result.Stream {
				received = append(received, part)
			}
			assert.Equal(t, parts, received)
			require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
			env.Shutdown(t)

			assert.Equal(t, 1, countModelLogEvent(t, logs.String(), "aisdk.model.stream.start"))
			assert.Equal(t, 1, countModelLogEvent(t, logs.String(), "aisdk.model.stream.finish"))
			assert.Contains(t, logs.String(), "ai_sdk.stream.time_to_first_content_ms")
			metrics := testMetrics(t, telemetry)
			assert.Equal(t, 1, strings.Count(metrics, "aisdk_model_requests_total{"))
			assert.Contains(t, metrics, "aisdk_model_time_to_first_output_seconds_count")
			generation := env.SingleGenerationJSON(t)
			if scenario == "normal_strongest_usage" {
				assert.Equal(t, "stop", generation["stop_reason"])
				assert.Equal(t, "5", testkit.StringValue(t, generation, "usage", "input_tokens"))
				assert.Equal(t, "3", testkit.StringValue(t, generation, "usage", "output_tokens"))
			} else {
				assert.NotContains(t, generation, "stop_reason")
			}
			encoded, err := json.Marshal(generation)
			require.NoError(t, err)
			assert.NotContains(t, logs.String()+metrics+string(encoded), "output-private")
			assert.NotContains(t, logs.String()+metrics+string(encoded), "finish-private")
		})
	}
}

func testMetrics(t *testing.T, telemetry *Telemetry) string {
	t.Helper()
	gatherer, ok := telemetry.Registerer().(prometheus.Gatherer)
	require.True(t, ok)
	response := httptest.NewRecorder()
	promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return response.Body.String()
}

func countModelLogEvent(t *testing.T, logs, value string) int {
	t.Helper()
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["ai_sdk.event"] == value {
			count++
		}
	}
	return count
}

type observabilityTestModel struct {
	generate func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	stream   func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (buffer *lockedBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Write(value)
}

func (buffer *lockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

func (*observabilityTestModel) SpecificationVersion() string               { return "v4" }
func (*observabilityTestModel) Provider() string                           { return "anthropic" }
func (*observabilityTestModel) ModelID() string                            { return "backend-private" }
func (*observabilityTestModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (model *observabilityTestModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	return model.generate(ctx, options)
}
func (model *observabilityTestModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	if model.stream == nil {
		return nil, nil
	}
	return model.stream(ctx, options)
}

const customContentRequestBody = `{"prompt":[{"role":"user","content":[{"type":"text","text":"private-prompt"}]},{"role":"assistant","content":[{"type":"custom","kind":"openai.compaction","providerOptions":{"openai":{"itemId":"private-metadata-item","encryptedContent":"private-metadata-encrypted"}}}]},{"role":"user","content":[{"type":"text","text":"private-prompt"}]}]}`

func TestCustomContentTelemetry_MetadataOnlyKeepsOptionsAndMetadataPrivate(t *testing.T) {
	customMetadata := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"private-metadata-out","encryptedContent":"private-metadata-encrypted-out"}`)}
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "unary", true: "stream"}[streaming], func(t *testing.T) {
			h := newFallbackAcceptance(t,
				&observabilityTestModel{
					generate: func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
						require.Len(t, options.Prompt, 3)
						return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentCustom, Kind: "openai.compaction", ProviderMetadata: customMetadata}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
					},
					stream: func(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
						require.Len(t, options.Prompt, 3)
						return fallbackParts(
							provider.StreamPart{Type: provider.PartCustom, Kind: "openai.compaction", ProviderMetadata: customMetadata},
							provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
						), nil
					},
				},
				&observabilityTestModel{},
			)
			response := httptest.NewRecorder()

			h.handler.ServeHTTP(response, h.request(context.Background(), streaming, customContentRequestBody))

			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), "private-metadata-out", "the caller keeps custom metadata")
			assert.Contains(t, response.Body.String(), "private-metadata-encrypted-out")
			mode := map[bool]string{false: "generate", true: "stream"}[streaming]
			h.verify(t, mode, response.Body.String(), []fallback.AttemptOutcome{fallback.AttemptSelected})
		})
	}
}

func TestCustomContentTelemetry_Disposition(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "unary", true: "stream"}[streaming], func(t *testing.T) {
			env := testkit.NewEnv(t, func(c *agento11y.Config) {
				c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
				c.Hooks = agento11y.HooksConfig{Enabled: false}
			})
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			telemetry, err := NewTelemetry(logger)
			require.NoError(t, err)
			runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
			factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
			require.NoError(t, err)
			metadata := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"metadata-private"}`)}
			lower := &observabilityTestModel{
				generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentCustom, Kind: "kind-private.custom", ProviderMetadata: metadata}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
				},
				stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					parts := make(chan provider.StreamPart, 3)
					parts <- provider.StreamPart{Type: provider.PartStreamStart}
					parts <- provider.StreamPart{Type: provider.PartCustom, Kind: "kind-private.custom", ProviderMetadata: metadata}
					parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				},
			}
			model, err := factory("grafana/assistant", lower)
			require.NoError(t, err)
			history := provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(func() provider.ContentPart {
				part := provider.CustomPart("kind-private.history")
				part.ProviderOptions = provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"itemId":"options-private"}`)}}
				return part
			}())}}
			if streaming {
				result, err := model.DoStream(context.Background(), history)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := model.DoGenerate(context.Background(), history)
				require.NoError(t, err)
			}
			require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
			env.Shutdown(t)
			generation, err := json.Marshal(env.SingleGenerationJSON(t))
			require.NoError(t, err)
			metrics := testMetrics(t, telemetry)

			for _, private := range []string{"metadata-private", "options-private"} {
				assert.NotContains(t, string(generation)+logs.String()+metrics, private)
			}
			assert.NotContains(t, string(generation), "kind-private", "Agent Observability omits custom parts")
			assert.NotContains(t, logs.String(), "kind-private", "operator logs carry no custom kinds under metadata-only capture")
			if streaming {
				assert.Contains(t, metrics, "aisdk_model_time_to_first_output_seconds_count", "Prometheus counts custom as first output")
			}
		})
	}
}

func TestModelObservationChain_SequentialSegments(t *testing.T) {
	inputFirst, outputFirst, inputSecond, outputSecond := 7, 2, 4, 9
	toolCalls := provider.FinishReason{Unified: provider.FinishReasonToolCalls}
	stop := provider.FinishReason{Unified: provider.FinishReasonStop}
	segments := []provider.StreamPart{
		{Type: provider.PartResponseMeta, ResponseID: "msg_1"},
		{Type: provider.PartTextStart, ID: "0"},
		{Type: provider.PartTextDelta, ID: "0", Delta: "one"},
		{Type: provider.PartTextEnd, ID: "0"},
		{Type: provider.PartFinish, FinishReason: &toolCalls, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &inputFirst}, OutputTokens: provider.OutputTokenUsage{Total: &outputFirst}}},
		{Type: provider.PartResponseMeta, ResponseID: "msg_2"},
		{Type: provider.PartTextStart, ID: "0"},
		{Type: provider.PartTextDelta, ID: "0", Delta: "two"},
		{Type: provider.PartTextEnd, ID: "0"},
		{Type: provider.PartFinish, FinishReason: &stop, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &inputSecond}, OutputTokens: provider.OutputTokenUsage{Total: &outputSecond}}},
	}
	providerError := provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "upstream overloaded", StatusCode: http.StatusServiceUnavailable})}
	for _, tc := range []struct {
		name            string
		parts           []provider.StreamPart
		stall           bool
		idle            time.Duration
		deadline        time.Duration
		wireErrors      int
		logEvent        string
		logOutcome      string
		metricLabels    string
		generationOut   int
		generationStop  provider.UnifiedFinishReason
		generationError string
	}{
		{name: "normal close", parts: segments, idle: 5 * time.Second, logEvent: "aisdk.model.stream.finish", logOutcome: "success", metricLabels: `error_type="none",finish_reason="stop"`, generationOut: outputSecond, generationStop: provider.FinishReasonStop},
		{name: "deadline stall after finish", parts: segments[:5], stall: true, idle: 5 * time.Second, deadline: 100 * time.Millisecond, wireErrors: 1, logEvent: "aisdk.model.stream.error", logOutcome: "timeout", metricLabels: `error_type="context_deadline_exceeded"`, generationOut: outputFirst, generationStop: provider.FinishReasonToolCalls},
		{name: "idle stall after finish", parts: segments[:5], stall: true, idle: 50 * time.Millisecond, wireErrors: 1, logEvent: "aisdk.model.stream.cancelled", logOutcome: "cancelled", metricLabels: `error_type="context_canceled"`, generationOut: outputFirst, generationStop: provider.FinishReasonToolCalls},
		{name: "provider error before finish, then deadline stall", parts: append([]provider.StreamPart{providerError}, segments[:5]...), stall: true, idle: 5 * time.Second, deadline: 100 * time.Millisecond, wireErrors: 2, logEvent: "aisdk.model.stream.error", logOutcome: "error", metricLabels: `error_type="context_deadline_exceeded"`, generationOut: outputFirst, generationStop: provider.FinishReasonToolCalls, generationError: "server_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testkit.NewEnv(t, func(configuration *agento11y.Config) {
				configuration.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
				configuration.Hooks = agento11y.HooksConfig{Enabled: false}
			})
			var logs lockedBuffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			telemetry, err := NewTelemetry(logger)
			require.NoError(t, err)
			runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
			factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
			require.NoError(t, err)
			model, err := factory("grafana/assistant", &observabilityTestModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				stream := make(chan provider.StreamPart, len(tc.parts))
				for _, part := range tc.parts {
					stream <- part
				}
				if tc.stall {
					go func() { <-ctx.Done(); close(stream) }()
				} else {
					close(stream)
				}
				return &provider.StreamResult{Stream: stream}, nil
			}})
			require.NoError(t, err)
			handler, err := newWireHandler(execution.Config{Selector: execution.CatalogSelector(&accountCatalogSpy{model: model}), Limits: execution.Limits{StreamIdleDuration: tc.idle, StreamDrainDuration: 100 * time.Millisecond}})
			require.NoError(t, err)
			ctx := t.Context()
			if tc.deadline > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.deadline)
				defer cancel()
			}
			request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[]}`)).WithContext(ctx)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
			request.Header.Set(providerv4.HeaderModelID, "configured-canonical")
			request.Header.Set(providerv4.HeaderStreaming, "true")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			body := response.Body.String()
			finishes := 2
			if tc.stall {
				finishes = 1
			}
			assert.Equal(t, finishes, strings.Count(body, `"type":"finish"`))
			assert.Equal(t, tc.wireErrors, strings.Count(body, `"type":"error"`))
			if tc.stall {
				assert.Contains(t, body, `"code":"timeout"`)
				assert.Less(t, strings.LastIndex(body, `"type":"finish"`), strings.LastIndex(body, `"type":"error"`))
			}

			require.Eventually(t, func() bool { return countModelLogEvent(t, logs.String(), tc.logEvent) == 1 }, time.Second, time.Millisecond)
			var finish map[string]any
			for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
				var record map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &record))
				if record["ai_sdk.event"] == tc.logEvent {
					finish = record
				}
			}
			assert.Equal(t, tc.logOutcome, finish["ai_sdk.outcome"])
			if !tc.stall {
				assert.Equal(t, float64(inputFirst), finish["ai_sdk.usage.input_tokens.total"])
				assert.Equal(t, float64(outputSecond), finish["ai_sdk.usage.output_tokens.total"])
				assert.Equal(t, string(provider.FinishReasonStop), finish["ai_sdk.finish_reason"])
			}
			metrics := testMetrics(t, telemetry)
			assert.Equal(t, 1, strings.Count(metrics, "aisdk_model_requests_total{"))
			assert.Contains(t, metrics, tc.metricLabels)
			status := `status="success"`
			if tc.stall {
				status = `status="canceled"`
			}
			assert.Contains(t, metrics, status)

			require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
			env.Shutdown(t)
			generation := env.SingleGenerationJSON(t)
			assert.Equal(t, strconv.Itoa(inputFirst), testkit.StringValue(t, generation, "usage", "input_tokens"))
			assert.Equal(t, strconv.Itoa(tc.generationOut), testkit.StringValue(t, generation, "usage", "output_tokens"))
			assert.Equal(t, string(tc.generationStop), generation["stop_reason"])
			if tc.generationError == "" {
				assert.Empty(t, generation["call_error"])
			} else {
				assert.Contains(t, generation["call_error"], tc.generationError)
			}
		})
	}
}
