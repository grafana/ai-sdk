package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type physicalBuffer struct{ lockedBuffer }

func (*physicalBuffer) SetWriteDeadline(time.Time) error { return nil }
func (*physicalBuffer) Close() error                     { return nil }

func TestPhysicalAttempts_AllowlistedProjectionAndImmutableIdentity(t *testing.T) {
	var output physicalBuffer
	var logs lockedBuffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(&output, telemetry, 8, time.Second, time.Second)
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
	sink.Close()
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 3)
	var records []physicalAttemptRecord
	for _, line := range lines {
		var record physicalAttemptRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
		assert.Contains(t, line, `"event":"gateway_physical_attempt"`)
	}
	assert.Equal(t, "primary-instance", records[0].ProviderInstance)
	assert.Equal(t, "backend-secondary", records[1].BackendModelID)
	assert.Equal(t, "correlation", records[0].CorrelationID)
	assert.Equal(t, "correlation", records[1].CorrelationID)
	assert.Empty(t, records[2].CorrelationID)
	assert.True(t, records[0].WillFallback)
	assert.True(t, records[1].Winner)
	for _, secret := range []string{"private-caller", "private-namespace", "secret.example", "private-header", "private-key", "private-body", "private-raw-error", "private-outcome", "mutated"} {
		assert.NotContains(t, output.String()+logs.String()+testMetrics(t, telemetry), secret)
	}
	assert.Contains(t, testMetrics(t, telemetry), `physical_attempt_dropped_total{class="invalid_record"} 2`)
	assert.Empty(t, logs.String())
}

func TestPhysicalAttempts_BoundedSaturationShutdownAndConcurrentClose(t *testing.T) {
	writer, reader := net.Pipe()
	defer func() { _ = reader.Close() }()
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(writer, telemetry, 2, 20*time.Millisecond, 30*time.Millisecond)
	started := time.Now()
	for range 10000 {
		sink.enqueue(physicalAttemptRecord{})
	}
	assert.Less(t, time.Since(started), time.Second)
	var work sync.WaitGroup
	for range 20 {
		work.Add(1)
		go func() { defer work.Done(); sink.enqueue(physicalAttemptRecord{}); sink.Close() }()
	}
	work.Wait()
	assert.Less(t, time.Since(started), time.Second)
	select {
	case <-sink.done:
	default:
		t.Fatal("worker survived shutdown")
	}
	metrics := testMetrics(t, telemetry)
	assert.Contains(t, metrics, `class="queue_full"`)
	assert.Contains(t, metrics, `class="transport"`)
	assert.Contains(t, metrics, `class="shutdown"`)
}

func TestPhysicalAttempts_RealPipeWriteDeadline(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("production FIFO output is Linux-only; portable worker bounds use net.Pipe")
	}
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()
	require.NoError(t, writer.SetWriteDeadline(time.Now().Add(10*time.Millisecond)))
	_, err = writer.Write(make([]byte, 4<<20))
	require.Error(t, err)
	sink := newPhysicalAttemptSink(writer, nil, 2, 10*time.Millisecond, 20*time.Millisecond)
	for range 3 {
		sink.enqueue(physicalAttemptRecord{})
	}
	started := time.Now()
	sink.Close()
	assert.Less(t, time.Since(started), time.Second)
	_, err = writer.Write([]byte("closed"))
	require.ErrorIs(t, err, os.ErrClosed)
}

type panicPhysicalOutput struct{}

func (panicPhysicalOutput) SetWriteDeadline(time.Time) error { return nil }
func (panicPhysicalOutput) Write([]byte) (int, error)        { panic("private-panic") }
func (panicPhysicalOutput) Close() error                     { return nil }

func TestPhysicalAttempts_WorkerPanicAndUnavailableOutputFailOpen(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(panicPhysicalOutput{}, telemetry, 2, time.Second, time.Second)
	sink.enqueue(physicalAttemptRecord{})
	sink.Close()
	assert.Contains(t, testMetrics(t, telemetry), `physical_attempt_dropped_total{class="worker"} 1`)
	disabled := newPhysicalAttemptSink(nil, telemetry, 2, time.Second, time.Second)
	disabled.enqueue(physicalAttemptRecord{})
	disabled.Close()
	var absent *PhysicalAttemptSink
	absent.enqueue(physicalAttemptRecord{})
	absent.Close()
}
