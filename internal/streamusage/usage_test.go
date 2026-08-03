package streamusage

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregator(t *testing.T) {
	t.Run("no usage observed", func(t *testing.T) {
		var aggregate Aggregator
		_, ok := aggregate.Usage()
		assert.False(t, ok)
	})

	t.Run("preserves strongest normalized fields independently", func(t *testing.T) {
		var aggregate Aggregator
		inputTotal, inputNoCache, cacheRead, cacheWrite := 120, 80, 30, 10
		outputReasoning := 20
		aggregate.Observe(provider.StreamPart{Type: provider.PartResponseMeta, Usage: &provider.Usage{
			InputTokens: provider.InputTokenUsage{
				Total: &inputTotal, NoCache: &inputNoCache, CacheRead: &cacheRead, CacheWrite: &cacheWrite,
			},
			OutputTokens: provider.OutputTokenUsage{Reasoning: &outputReasoning},
			Raw:          json.RawMessage(`{"sequence":1}`),
		}})

		outputTotal, outputText := 50, 30
		aggregate.Observe(provider.StreamPart{Type: provider.PartTextDelta, Usage: &provider.Usage{
			OutputTokens: provider.OutputTokenUsage{Total: &outputTotal, Text: &outputText},
		}})

		provisionalInput, provisionalRead, provisionalWrite := 100, 20, 5
		provisionalOutput, provisionalText, provisionalReasoning := 45, 25, 15
		aggregate.Observe(provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{
			InputTokens: provider.InputTokenUsage{
				Total: &provisionalInput, CacheRead: &provisionalRead, CacheWrite: &provisionalWrite,
			},
			OutputTokens: provider.OutputTokenUsage{
				Total: &provisionalOutput, Text: &provisionalText, Reasoning: &provisionalReasoning,
			},
		}})

		usage, ok := aggregate.Usage()
		require.True(t, ok)
		assert.Equal(t, inputTotal, *usage.InputTokens.Total)
		assert.Equal(t, inputNoCache, *usage.InputTokens.NoCache)
		assert.Equal(t, cacheRead, *usage.InputTokens.CacheRead)
		assert.Equal(t, cacheWrite, *usage.InputTokens.CacheWrite)
		assert.Equal(t, outputTotal, *usage.OutputTokens.Total)
		assert.Equal(t, outputText, *usage.OutputTokens.Text)
		assert.Equal(t, outputReasoning, *usage.OutputTokens.Reasoning)
		assert.JSONEq(t, `{"sequence":1}`, string(usage.Raw))
	})

	t.Run("latest non-empty raw usage wins", func(t *testing.T) {
		var aggregate Aggregator
		aggregate.Observe(provider.StreamPart{Usage: &provider.Usage{Raw: json.RawMessage(`{"sequence":1}`)}})
		aggregate.Observe(provider.StreamPart{Usage: &provider.Usage{}})
		aggregate.Observe(provider.StreamPart{Usage: &provider.Usage{Raw: json.RawMessage(`{"sequence":3}`)}})

		usage, ok := aggregate.Usage()
		require.True(t, ok)
		assert.JSONEq(t, `{"sequence":3}`, string(usage.Raw))
	})
}
