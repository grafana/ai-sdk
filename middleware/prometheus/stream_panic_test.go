package prometheus

import (
	"context"
	"testing"

	"github.com/grafana/ai-sdk/internal/streamrecover"
	"github.com/grafana/ai-sdk/provider"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func panicTestStream(parts ...provider.StreamPart) *mockModel {
	return &mockModel{providerName: "test", modelID: "model", streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		ch := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			ch <- part
		}
		close(ch)
		return &provider.StreamResult{Stream: ch}, nil
	}}
}

func TestDoStream_FinalizePanic(t *testing.T) {
	reg := promclient.NewRegistry()
	parts := []provider.StreamPart{
		{Type: provider.PartResponseMeta, Provider: "test", ModelID: "response-model"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}},
	}
	wrapped, err := Wrap(panicTestStream(parts...), Options{Registerer: reg, NormalizeModel: func(_, model string) string {
		if model == "response-model" {
			panic("secret-panic-value")
		}
		return model
	}})
	require.NoError(t, err)
	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	var got []provider.StreamPart
	for part := range result.Stream {
		got = append(got, part)
	}
	assert.Equal(t, parts, got)
	gauge := findMetric(t, reg, metricInflightRequests, map[string]string{"operation": operationStream})
	assert.Zero(t, gauge.GetGauge().GetValue())
}

func TestDoStream_RecoveredPanicErrorType(t *testing.T) {
	reg := promclient.NewRegistry()
	wrapped, err := Wrap(panicTestStream(streamrecover.Part()), Options{Registerer: reg})
	require.NoError(t, err)
	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	for range result.Stream {
	}
	assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationStream, "error_type": errorTypeOther}, 1)
}
