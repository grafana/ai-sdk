package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRawOutputMetadataOnlyObservation(t *testing.T) {
	env := testkit.NewEnv(t, func(c *agento11y.Config) {
		c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
		c.Hooks = agento11y.HooksConfig{Enabled: false}
	})
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)
	raw := json.RawMessage(`{"type":"response.created","response":{"id":"raw-response-private","output_text":"raw-text-private"}}`)
	lower := &observabilityTestModel{
		stream: func(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
			assert.True(t, options.IncludeRawChunks)
			parts := make(chan provider.StreamPart, 4)
			parts <- provider.StreamPart{Type: provider.PartStreamStart}
			parts <- provider.StreamPart{Type: provider.PartRaw, RawValue: raw}
			parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
			close(parts)
			return &provider.StreamResult{Stream: parts}, nil
		},
	}
	model, err := factory("grafana/assistant", lower)
	require.NoError(t, err)
	result, err := model.DoStream(context.Background(), provider.CallOptions{IncludeRawChunks: true})
	require.NoError(t, err)
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	require.Len(t, parts, 3)
	assert.Equal(t, raw, parts[1].RawValue)
	require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
	env.Shutdown(t)
	generation := env.SingleGenerationJSON(t)
	encoded, err := json.Marshal(generation)
	require.NoError(t, err)
	public := string(encoded) + logs.String() + testMetrics(t, telemetry)
	for _, private := range []string{"raw-response-private", "raw-text-private"} {
		assert.NotContains(t, public, private)
	}
}
