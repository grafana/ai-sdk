package agentobservability

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRecordingMiddleware_InclusiveUsageExport(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "generate", true: "stream"}[streaming], func(t *testing.T) {
			env := testkit.NewEnv(t)
			usage := provider.Usage{
				InputTokens:  provider.InputTokenUsage{Total: intPtr(60), NoCache: intPtr(10), CacheRead: intPtr(30), CacheWrite: intPtr(20)},
				OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)},
			}
			model := &mockLanguageModel{provider_: "anthropic", modelID: "claude",
				doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hi"}}, Usage: usage}, nil
				},
				doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					parts := make(chan provider.StreamPart, 3)
					parts <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "hi", Usage: &usage}
					provisional := provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(1)}}
					parts <- provider.StreamPart{Type: provider.PartFinish, Usage: &provisional}
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				},
			}
			wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
				ClientResolver: func(context.Context) *agento11y.Client { return env.Client },
			})}})
			params := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
			if streaming {
				result, err := wrapped.DoStream(context.Background(), params)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := wrapped.DoGenerate(context.Background(), params)
				require.NoError(t, err)
			}
			require.Eventually(t, func() bool {
				return len(env.Spans.Ended()) == 1
			}, 2*time.Second, 10*time.Millisecond)
			require.NoError(t, env.Client.Flush(context.Background()))
			generation := env.SingleGenerationJSON(t)
			assert.Equal(t, "TOKEN_INPUT_SEMANTICS_INCLUSIVE", testkit.StringValue(t, generation, "usage", "input_semantics"))
			assert.Equal(t, "60", testkit.StringValue(t, generation, "usage", "input_tokens"))
			assert.Equal(t, "65", testkit.StringValue(t, generation, "usage", "total_tokens"))
			spans := env.Spans.Ended()
			require.Len(t, spans, 1)
			attrs := spanAttributes(spans[0])
			assert.Equal(t, "inclusive", attrs["gen_ai.token.semantics"])
			assert.Equal(t, "60", attrs["gen_ai.usage.input_tokens"])

			var metrics metricdata.ResourceMetrics
			require.NoError(t, env.Metrics.Collect(context.Background(), &metrics))
			counts := map[string]int64{}
			for _, scope := range metrics.ScopeMetrics {
				for _, metric := range scope.Metrics {
					if metric.Name != "gen_ai.client.token.usage" {
						continue
					}
					histogram, ok := metric.Data.(metricdata.Histogram[int64])
					require.True(t, ok)
					for _, point := range histogram.DataPoints {
						semantics, ok := point.Attributes.Value(attribute.Key("gen_ai.token.semantics"))
						require.True(t, ok)
						assert.Equal(t, "inclusive", semantics.AsString())
						tokenType, ok := point.Attributes.Value(attribute.Key("gen_ai.token.type"))
						require.True(t, ok)
						assert.Equal(t, uint64(1), point.Count)
						counts[tokenType.AsString()] = point.Sum
					}
				}
			}
			assert.Equal(t, map[string]int64{"input": 60, "output": 5, "cache_read": 30, "cache_write": 20}, counts)
		})
	}
}
