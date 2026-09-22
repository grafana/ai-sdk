package prometheus

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDrainStreamUntil(t *testing.T) {
	for _, mode := range []string{"closed", "silent", "ready", "expired"} {
		t.Run(mode, func(t *testing.T) {
			upstream := make(chan provider.StreamPart, 1024)
			if mode == "closed" {
				close(upstream)
			}
			if mode == "ready" {
				for range cap(upstream) {
					upstream <- provider.StreamPart{}
				}
				stop := make(chan struct{})
				producerDone := make(chan struct{})
				go func() {
					defer close(producerDone)
					for {
						select {
						case upstream <- provider.StreamPart{}:
						case <-stop:
							return
						}
					}
				}()
				defer func() { close(stop); <-producerDone }()
			}
			deadline := time.Now().Add(20 * time.Millisecond)
			if mode == "expired" {
				deadline = time.Now().Add(-time.Second)
			}
			done := make(chan struct{})
			go func() { drainStreamUntil(upstream, deadline); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("drain retained goroutine beyond deadline")
			}
		})
	}
}

func TestStreamMetrics_BoundedDrain(t *testing.T) {
	for _, mode := range []string{"silent", "buffered cancellation race"} {
		t.Run(mode, func(t *testing.T) {
			reg := promclient.NewRegistry()
			upstream := make(chan provider.StreamPart, streamBufferSize+1)
			defer close(upstream)
			if mode != "silent" {
				for range cap(upstream) {
					upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "text"}
				}
			}
			request := &provider.RequestMetadata{}
			response := &provider.ResponseHeaders{}
			model := &mockModel{providerName: "grafana", modelID: "grafana/assistant", streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: upstream, Request: request, Response: response}, nil
			}}
			wrapped, err := Wrap(model, Options{Registerer: reg, StreamDrainTimeout: time.Second})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result, err := wrapped.DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)
			assert.Same(t, request, result.Request)
			assert.Same(t, response, result.Response)
			cancel()
			done := make(chan struct{})
			go func() {
				for range result.Stream {
				}
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(250 * time.Millisecond):
				t.Fatal("tee finalization waited for drain")
			}
			assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationStream, "status": statusCanceled}, 1)
			metric := findMetric(t, reg, metricInflightRequests, map[string]string{"operation": operationStream})
			assert.Equal(t, float64(0), metric.GetGauge().GetValue())
		})
	}
}

func TestDrainCanceledStream_ZeroDrainsUntilClose(t *testing.T) {
	upstream := make(chan provider.StreamPart)
	defer close(upstream)
	i := &instrumentation{config: normalizeOptions(Options{})}
	i.drainCanceledStream(upstream)
	for range 3 {
		select {
		case upstream <- provider.StreamPart{}:
		case <-time.After(time.Second):
			t.Fatal("default drain stopped before upstream closed")
		}
	}
}

func TestStreamMetrics_BoundedDrainCancellationMatrix(t *testing.T) {
	for _, mode := range []string{"full-output", "already-canceled", "deadline", "ready", "canceled-close"} {
		t.Run(mode, func(t *testing.T) {
			reg := promclient.NewRegistry()
			upstream := make(chan provider.StreamPart)
			if mode != "ready" && mode != "canceled-close" {
				defer close(upstream)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer deadlineCancel()
			}
			if mode == "already-canceled" {
				cancel()
			}
			model := &mockModel{providerName: "grafana", modelID: "grafana/assistant", streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: upstream}, nil
			}}
			wrapped, err := Wrap(model, Options{Registerer: reg, StreamDrainTimeout: 2 * time.Second})
			require.NoError(t, err)
			result, err := wrapped.DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)
			if mode == "full-output" {
				for range streamBufferSize + 1 {
					select {
					case upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "x"}:
					case <-time.After(time.Second):
						t.Fatal("tee did not fill")
					}
				}
			}
			if mode == "ready" {
				stop, producerDone := make(chan struct{}), make(chan struct{})
				go func() {
					defer close(producerDone)
					defer close(upstream)
					for {
						select {
						case upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "x"}:
						case <-stop:
							return
						}
					}
				}()
				t.Cleanup(func() { close(stop); <-producerDone })
				select {
				case <-result.Stream:
				case <-time.After(time.Second):
					t.Fatal("ready producer produced no output")
				}
			}
			if mode == "canceled-close" {
				cancel()
				close(upstream)
			} else if mode != "deadline" {
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
			case <-time.After(time.Second):
				t.Fatal("tee waited for cleanup")
			}
			families, err := reg.Gather()
			require.NoError(t, err)
			requests := findFamily(families, metricRequestsTotal)
			require.NotNil(t, requests)
			var total float64
			for _, metric := range requests.GetMetric() {
				total += metric.GetCounter().GetValue()
			}
			assert.Equal(t, float64(1), total)
			metric := findMetric(t, reg, metricInflightRequests, map[string]string{"operation": operationStream})
			assert.Zero(t, metric.GetGauge().GetValue())
			if mode != "deadline" {
				assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationStream, "status": statusCanceled}, 1)
			}
		})
	}
}
