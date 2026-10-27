package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"

	backendlog "github.com/grafana/grafana-plugin-sdk-go/backend/log"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/pluginlog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type hclogAdapter struct{ logger hclog.Logger }

func (a hclogAdapter) Debug(msg string, args ...any) { a.logger.Debug(msg, args...) }
func (a hclogAdapter) Info(msg string, args ...any)  { a.logger.Info(msg, args...) }
func (a hclogAdapter) Warn(msg string, args ...any)  { a.logger.Warn(msg, args...) }
func (a hclogAdapter) Error(msg string, args ...any) { a.logger.Error(msg, args...) }
func (a hclogAdapter) With(args ...any) backendlog.Logger {
	return hclogAdapter{logger: a.logger.With(args...)}
}
func (a hclogAdapter) Level() backendlog.Level                       { return backendlog.Info }
func (a hclogAdapter) FromContext(context.Context) backendlog.Logger { return a }

func hclogLogger(output io.Writer) *slog.Logger {
	return slog.New(pluginlog.NewHandler(hclogAdapter{logger: hclog.New(&hclog.LoggerOptions{Output: output, JSONFormat: true, Level: hclog.Info})}, slog.LevelInfo))
}

func TestPhysicalAttempts_AllowlistedProjectionAndImmutableIdentity(t *testing.T) {
	var output lockedBuffer
	var logs lockedBuffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(hclogLogger(&output), telemetry, 8)
	descriptors := []config.Primary{{Provider: "primary-instance", Model: "backend-primary"}, {Provider: "secondary-instance", Model: "backend-secondary"}}
	observer := physicalAttemptObserver(descriptors, sink)
	descriptors[0].Provider = "mutated"
	ctx := context.WithValue(context.Background(), modelObservationKey{}, requestObservation{correlationID: "correlation", callerService: "private-caller", namespace: "private-namespace"})
	now := time.Now()
	attempt := fallback.Attempt{Index: 1, Provider: "https://secret.example", ModelID: "private-header", StartedAt: now, FinishedAt: now.Add(time.Millisecond), Outcome: fallback.AttemptFailed, WillFallback: true, Err: errors.New("private-key private-body private-raw-error")}
	observer(ctx, attempt)
	attempt.Index, attempt.Outcome, attempt.WillFallback = 2, fallback.AttemptSelected, false
	observer(ctx, attempt)
	observer(context.Background(), attempt)
	attempt.Index = 0
	observer(ctx, attempt)
	attempt.Index, attempt.Outcome = 2, "private-outcome"
	observer(ctx, attempt)
	sink.Close(context.Background())
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 3)
	var records []physicalAttemptRecord
	for _, line := range lines {
		var record physicalAttemptRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
		assert.Contains(t, line, `"event":"gateway_physical_attempt"`)
		assert.Contains(t, line, `"@level":"info"`)
		assert.Contains(t, line, `"@message":"gateway physical attempt"`)
	}
	assert.Equal(t, "primary-instance", records[0].ProviderInstance)
	assert.Equal(t, "backend-secondary", records[1].BackendModelID)
	assert.Equal(t, "correlation", records[0].CorrelationID)
	assert.Equal(t, "correlation", records[1].CorrelationID)
	assert.Empty(t, records[2].CorrelationID)
	assert.True(t, records[0].WillFallback)
	assert.True(t, records[1].Winner)
	assert.False(t, records[0].StartedAt.IsZero())
	for _, secret := range []string{"private-caller", "private-namespace", "secret.example", "private-header", "private-key", "private-body", "private-raw-error", "private-outcome", "mutated"} {
		assert.NotContains(t, output.String()+logs.String()+testMetrics(t, telemetry), secret)
	}
	assert.Contains(t, testMetrics(t, telemetry), `physical_attempt_dropped_total{class="invalid_record"} 2`)
	assert.Empty(t, logs.String())
}

func TestPhysicalAttempts_StalledOutputSaturationAndClose(t *testing.T) {
	writer, reader := net.Pipe()
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(hclogLogger(writer), telemetry, 2)
	started := time.Now()
	for range 10000 {
		sink.enqueue(physicalAttemptRecord{ProviderInstance: "primary", BackendModelID: "model"})
	}
	assert.Less(t, time.Since(started), time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var closing sync.WaitGroup
	for range 20 {
		closing.Go(func() {
			sink.enqueue(physicalAttemptRecord{})
			sink.Close(ctx)
		})
	}
	closing.Wait()
	assert.Less(t, time.Since(started), time.Second)
	select {
	case <-sink.done:
		t.Fatal("the stalled worker should have been abandoned, not drained")
	default:
	}

	require.NoError(t, reader.Close())
	select {
	case <-sink.done:
	case <-time.After(2 * time.Second):
		t.Fatal("abandoned worker did not stop after its output failed")
	}
	metrics := testMetrics(t, telemetry)
	assert.Contains(t, metrics, `class="queue_full"`)
	assert.Contains(t, metrics, `class="shutdown"`)
}

type panicHandler struct{ slog.Handler }

func (panicHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (panicHandler) Handle(context.Context, slog.Record) error { panic("private-panic") }

func TestPhysicalAttempts_WorkerPanicAndUnavailableOutputFailOpen(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(slog.New(panicHandler{}), telemetry, 2)
	sink.enqueue(physicalAttemptRecord{})
	sink.Close(context.Background())
	assert.Contains(t, testMetrics(t, telemetry), `physical_attempt_dropped_total{class="worker"} 1`)
	disabled := newPhysicalAttemptSink(nil, telemetry, 2)
	disabled.enqueue(physicalAttemptRecord{})
	disabled.Close(context.Background())
	var absent *PhysicalAttemptSink
	absent.enqueue(physicalAttemptRecord{})
	absent.Close(context.Background())
}

func TestPhysicalAttempts_SaturationDoesNotSuppressConsumerEvidence(t *testing.T) {
	writer, reader := net.Pipe()
	defer func() { _ = reader.Close() }()
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(hclogLogger(writer), telemetry, 1)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		sink.Close(ctx)
	}()
	for range 10000 {
		sink.enqueue(physicalAttemptRecord{})
	}
	require.Contains(t, testMetrics(t, telemetry), `class="queue_full"`)
	created, err := buildCatalog(fallbackCatalogFile(), fallbackProviders(), http.DefaultClient, func(_ nativemodel.Config, id string, _ *http.Client) provider.LanguageModel {
		return &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			if id == "backend-primary" {
				return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "source failure"})
			}
			return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
		}}
	}, identityModelFactory, sink)
	require.NoError(t, err)
	handler, err := newWireHandler(execution.Config{Selector: execution.CatalogSelector(created), Limits: serviceTestLimits()})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, "alias")
	request.Header.Set(providerv4.HeaderStreaming, "false")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, 200, response.Code)
	assert.Contains(t, response.Body.String(), `"outcome":"selected"`)
	assert.Contains(t, response.Body.String(), `"message":"source failure"`)
	assert.Contains(t, response.Body.String(), `"outcome":"failed"`)
}
