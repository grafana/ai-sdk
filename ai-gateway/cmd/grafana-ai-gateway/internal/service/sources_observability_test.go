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
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestSourcesMetadataOnlyObservation(t *testing.T) {
	testSourcesMetadataOnlyObservation(t, false)
}

func testSourcesMetadataOnlyObservation(t *testing.T, requireAdoption bool) {
	t.Helper()
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
			runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
			factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
			require.NoError(t, err)
			source := provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "native-private", Title: "title-private", MediaType: "text/plain", Filename: "filename-private", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"fileId":"metadata-private"}`)}}
			lower := &observabilityTestModel{
				generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentSource, SourceType: source.SourceType, ID: source.ID, Text: source.Title, MediaType: source.MediaType, Filename: source.Filename, ProviderMetadata: source.ProviderMetadata}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
				},
				stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					parts := make(chan provider.StreamPart, 2)
					parts <- provider.StreamPart{Type: provider.PartSource, Source: &source}
					parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				},
			}
			model, err := factory("grafana/assistant", lower)
			require.NoError(t, err)
			if streaming {
				result, err := model.DoStream(context.Background(), provider.CallOptions{})
				require.NoError(t, err)
				var parts []provider.StreamPart
				for part := range result.Stream {
					parts = append(parts, part)
				}
				require.Len(t, parts, 2)
				assert.Equal(t, source, *parts[0].Source)
			} else {
				result, err := model.DoGenerate(context.Background(), provider.CallOptions{})
				require.NoError(t, err)
				require.Len(t, result.Content, 1)
				assert.Equal(t, source.ID, result.Content[0].ID)
			}
			require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
			if streaming && requireAdoption {
				assert.Contains(t, logs.String(), "ai_sdk.stream.time_to_first_content_ms")
				assert.Contains(t, testMetrics(t, telemetry), "aisdk_model_time_to_first_output_seconds_count")
				var metrics metricdata.ResourceMetrics
				require.NoError(t, env.Metrics.Collect(context.Background(), &metrics))
				var firstOutputCount uint64
				for _, scope := range metrics.ScopeMetrics {
					for _, metric := range scope.Metrics {
						if metric.Name == "gen_ai.client.time_to_first_token" {
							histogram, ok := metric.Data.(metricdata.Histogram[float64])
							require.True(t, ok)
							for _, point := range histogram.DataPoints {
								firstOutputCount += point.Count
							}
						}
					}
				}
				assert.Equal(t, uint64(1), firstOutputCount, "Agent Observability must record source-only first output")
			}
			env.Shutdown(t)
			generation := env.SingleGenerationJSON(t)
			encoded, err := json.Marshal(generation)
			require.NoError(t, err)
			public := string(encoded) + logs.String() + testMetrics(t, telemetry)
			for _, private := range []string{"native-private", "title-private", "filename-private", "metadata-private"} {
				assert.NotContains(t, public, private)
			}
		})
	}
}
