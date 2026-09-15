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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/middleware"
	agentmiddleware "github.com/grafana/ai-sdk/middleware/agentobservability"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAgentObservabilityRuntime_DisabledDoesNotResolveSecret(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.NoError(t, err)
	calls := 0
	runtime, err := NewAgentObservabilityRuntime(config.AgentObservabilitySettings{}, func(string) (string, bool) {
		calls++
		return "private", true
	}, telemetry)
	require.NoError(t, err)
	require.NotNil(t, runtime)
	assert.Nil(t, runtime.client)
	assert.Zero(t, calls)
	runtime.Close()
}

func TestAgentObservabilityRuntime_CloseWaitsForAcquiredRecordings(t *testing.T) {
	env := testkit.NewEnv(t)
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{
		client:          env.Client,
		telemetry:       telemetry,
		flushTimeout:    time.Second,
		shutdownTimeout: time.Second,
	}
	require.Same(t, env.Client, runtime.acquireClient(context.Background()))
	require.Same(t, env.Client, runtime.acquireClient(context.Background()))
	closed := make(chan struct{})
	go func() {
		runtime.Close()
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

func TestAgentObservabilityRuntime_CloseBoundsRecordingWait(t *testing.T) {
	env := testkit.NewEnv(t)
	var logs lockedBuffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{
		client:          env.Client,
		telemetry:       telemetry,
		flushTimeout:    10 * time.Millisecond,
		shutdownTimeout: time.Second,
	}
	require.Same(t, env.Client, runtime.acquireClient(context.Background()))
	started := time.Now()
	runtime.Close()
	assert.Less(t, time.Since(started), time.Second)
	assert.Contains(t, logs.String(), `"class":"shutdown"`)
	runtime.recordingComplete()
}

func TestAgentObservabilityRuntime_ConcurrentAcquireAndClose(t *testing.T) {
	env := testkit.NewEnv(t)
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{
		client:          env.Client,
		telemetry:       telemetry,
		flushTimeout:    time.Second,
		shutdownTimeout: time.Second,
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
		runtime.Close()
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
		client:          env.Client,
		telemetry:       telemetry,
		flushTimeout:    time.Second,
		shutdownTimeout: time.Second,
	}
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
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
		runtime.Close()
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

func TestNewAgentObservabilityRuntime_RejectsActualSDKEnvironment(t *testing.T) {
	t.Setenv("AGENTO11Y_TAGS", "private=ambient")
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.NoError(t, err)
	settings := config.AgentObservabilitySettings{Enabled: true, AuthSecretEnv: "AO_SECRET"}
	runtime, err := NewAgentObservabilityRuntime(settings, func(name string) (string, bool) {
		if name == "AO_SECRET" {
			return "private-secret", true
		}
		return "", false
	}, telemetry)
	require.Error(t, err)
	assert.Nil(t, runtime)
	assert.EqualError(t, err, "config: ambient agent observability SDK environment is not allowed")
	assert.NotContains(t, err.Error(), "AGENTO11Y_TAGS")
	assert.NotContains(t, err.Error(), "private=ambient")
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
	secretCalls := 0
	runtime, err := NewAgentObservabilityRuntime(config.AgentObservabilitySettings{
		Enabled:         true,
		Protocol:        config.AgentObservabilityHTTP,
		Endpoint:        server.URL,
		TLS:             false,
		AuthSecretEnv:   "AO_SECRET",
		QueueSize:       8,
		BatchSize:       1,
		PayloadMaxBytes: 1 << 20,
		MaxRetries:      1,
		InitialBackoff:  time.Millisecond,
		MaxBackoff:      time.Millisecond,
		FlushInterval:   time.Hour,
		FlushTimeout:    time.Second,
		ShutdownTimeout: time.Second,
	}, func(name string) (string, bool) {
		if name == "AO_SECRET" {
			secretCalls++
			return "agent-secret", true
		}
		return "", false
	}, telemetry)
	require.NoError(t, err)
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
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
	runtime.Close()
	assert.Equal(t, 1, secretCalls)
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
	runtime, err := NewAgentObservabilityRuntime(config.AgentObservabilitySettings{
		Enabled:         true,
		Protocol:        config.AgentObservabilityHTTP,
		Endpoint:        server.URL,
		TLS:             false,
		AuthSecretEnv:   "AO_SECRET",
		QueueSize:       4,
		BatchSize:       1,
		PayloadMaxBytes: 1 << 20,
		MaxRetries:      1,
		InitialBackoff:  time.Millisecond,
		MaxBackoff:      time.Millisecond,
		FlushInterval:   time.Hour,
		FlushTimeout:    time.Second,
		ShutdownTimeout: time.Second,
	}, func(name string) (string, bool) {
		return "agent-secret", name == "AO_SECRET"
	}, telemetry)
	require.NoError(t, err)
	defer runtime.Close()
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
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
			"gateway.region":         "us-central1",
			"provider.private":       "provider-option-private",
			"gateway.namespace":      strings.Repeat("x", maxObservationValueLen+1),
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
	assert.Equal(t, generation.Usage, filtered.Usage)
	assert.Equal(t, generation.SystemPrompt, filtered.SystemPrompt)
	assert.Equal(t, map[string]any{
		"gateway.correlation_id": "correlation",
		"gateway.region":         "us-central1",
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
	factory, err := NewModelObservabilityFactory(telemetry, logger, nil, 10*time.Millisecond)
	require.NoError(t, err)
	_, err = NewModelObservabilityFactory(telemetry, logger, nil, 10*time.Millisecond)
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
	factory, err := NewModelObservabilityFactory(telemetry, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, 10*time.Millisecond)
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
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)

	inputTokens, outputTokens := 2, 3
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "output-private"}},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: &inputTokens},
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
		"anthropic", "backend-private", "header-private", "body-private", "bearer-private", "2048", "web_search_requests", "topology-private",
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
	assert.Equal(t, "2", testkit.StringValue(t, generation, "usage", "input_tokens"))
	assert.Equal(t, "3", testkit.StringValue(t, generation, "usage", "output_tokens"))
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
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
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
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
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
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)

	inputTokens, outputTokens := 4, 6
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
		{Type: provider.PartTextDelta, ID: "text", Delta: "output-private", Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &inputTokens}}},
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
	assert.Equal(t, "4", testkit.StringValue(t, generation, "usage", "input_tokens"))
	assert.Equal(t, "6", testkit.StringValue(t, generation, "usage", "output_tokens"))
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
			runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
			const drainTimeout = 10 * time.Millisecond
			factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, drainTimeout)
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
			runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
			factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
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
	response := httptest.NewRecorder()
	telemetry.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
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
