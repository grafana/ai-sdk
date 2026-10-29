package streamusage

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tokens(input, output int) *provider.Usage {
	return &provider.Usage{
		InputTokens:  provider.InputTokenUsage{Total: &input},
		OutputTokens: provider.OutputTokenUsage{Total: &output},
	}
}

func observeAll(parts ...provider.StreamPart) Aggregator {
	var aggregate Aggregator
	for _, part := range parts {
		aggregate.Observe(part)
	}
	return aggregate
}

func TestAggregator(t *testing.T) {
	t.Run("no usage observed", func(t *testing.T) {
		var aggregate Aggregator
		_, ok := aggregate.Usage()
		assert.False(t, ok)
	})

	t.Run("before any finish, preserves strongest normalized fields independently", func(t *testing.T) {
		inputTotal, inputNoCache, cacheRead, cacheWrite := 120, 80, 30, 10
		outputTotal, outputText, outputReasoning := 50, 30, 20
		lowerInput, lowerOutput := 100, 45
		aggregate := observeAll(
			provider.StreamPart{Type: provider.PartResponseMeta, Usage: &provider.Usage{
				InputTokens: provider.InputTokenUsage{
					Total: &inputTotal, NoCache: &inputNoCache, CacheRead: &cacheRead, CacheWrite: &cacheWrite,
				},
				OutputTokens: provider.OutputTokenUsage{Reasoning: &outputReasoning},
			}},
			provider.StreamPart{Type: provider.PartTextDelta, Usage: &provider.Usage{
				OutputTokens: provider.OutputTokenUsage{Total: &outputTotal, Text: &outputText},
			}},
			provider.StreamPart{Type: provider.PartError, Usage: tokens(lowerInput, lowerOutput)},
		)

		usage, ok := aggregate.Usage()
		require.True(t, ok)
		assert.Equal(t, inputTotal, *usage.InputTokens.Total)
		assert.Equal(t, inputNoCache, *usage.InputTokens.NoCache)
		assert.Equal(t, cacheRead, *usage.InputTokens.CacheRead)
		assert.Equal(t, cacheWrite, *usage.InputTokens.CacheWrite)
		assert.Equal(t, outputTotal, *usage.OutputTokens.Total)
		assert.Equal(t, outputText, *usage.OutputTokens.Text)
		assert.Equal(t, outputReasoning, *usage.OutputTokens.Reasoning)
	})

	t.Run("finish counters replace provisional ones and omitted counters keep the segment's strongest", func(t *testing.T) {
		inputTotal, inputNoCache, outputTotal, outputText := 120, 80, 50, 30
		finishInput, finishOutput := 100, 45
		aggregate := observeAll(
			provider.StreamPart{Type: provider.PartResponseMeta, Usage: &provider.Usage{
				InputTokens: provider.InputTokenUsage{Total: &inputTotal, NoCache: &inputNoCache},
				Raw:         json.RawMessage(`{"sequence":1}`),
			}},
			provider.StreamPart{Type: provider.PartTextDelta, Usage: &provider.Usage{
				OutputTokens: provider.OutputTokenUsage{Total: &outputTotal, Text: &outputText},
			}},
			provider.StreamPart{Type: provider.PartFinish, Usage: tokens(finishInput, finishOutput)},
		)

		usage, ok := aggregate.Usage()
		require.True(t, ok)
		assert.Equal(t, finishInput, *usage.InputTokens.Total)
		assert.Equal(t, finishOutput, *usage.OutputTokens.Total)
		assert.Equal(t, inputNoCache, *usage.InputTokens.NoCache)
		assert.Equal(t, outputText, *usage.OutputTokens.Text)
		assert.JSONEq(t, `{"sequence":1}`, string(usage.Raw))
	})

	t.Run("a finish without usage closes the segment with its strongest counters", func(t *testing.T) {
		aggregate := observeAll(
			provider.StreamPart{Type: provider.PartResponseMeta, Usage: tokens(5, 1)},
			provider.StreamPart{Type: provider.PartFinish},
		)

		usage, ok := aggregate.Usage()
		require.True(t, ok)
		assert.Equal(t, 5, *usage.InputTokens.Total)
		assert.Equal(t, 1, *usage.OutputTokens.Total)
	})

	t.Run("the last finish's segment wins across several finishes", func(t *testing.T) {
		aggregate := observeAll(
			provider.StreamPart{Type: provider.PartResponseMeta, Usage: tokens(3369, 5)},
			provider.StreamPart{Type: provider.PartFinish, Usage: tokens(3369, 725)},
			provider.StreamPart{Type: provider.PartResponseMeta, Usage: tokens(4551, 725)},
			provider.StreamPart{Type: provider.PartFinish, Usage: tokens(4551, 197)},
		)

		usage, ok := aggregate.Usage()
		require.True(t, ok)
		assert.Equal(t, 4551, *usage.InputTokens.Total)
		assert.Equal(t, 197, *usage.OutputTokens.Total)
	})

	t.Run("usage after the last finish does not change the aggregate", func(t *testing.T) {
		aggregate := observeAll(
			provider.StreamPart{Type: provider.PartFinish, Usage: tokens(7, 2)},
			provider.StreamPart{Type: provider.PartResponseMeta, Usage: tokens(40, 90)},
			provider.StreamPart{Type: provider.PartError, Usage: tokens(40, 90)},
		)

		usage, ok := aggregate.Usage()
		require.True(t, ok)
		assert.Equal(t, 7, *usage.InputTokens.Total)
		assert.Equal(t, 2, *usage.OutputTokens.Total)
	})

	t.Run("latest non-empty raw usage wins before a finish", func(t *testing.T) {
		aggregate := observeAll(
			provider.StreamPart{Usage: &provider.Usage{Raw: json.RawMessage(`{"sequence":1}`)}},
			provider.StreamPart{Usage: &provider.Usage{}},
			provider.StreamPart{Usage: &provider.Usage{Raw: json.RawMessage(`{"sequence":3}`)}},
		)

		usage, ok := aggregate.Usage()
		require.True(t, ok)
		assert.JSONEq(t, `{"sequence":3}`, string(usage.Raw))
	})

	t.Run("finish raw usage replaces the segment's raw usage", func(t *testing.T) {
		aggregate := observeAll(
			provider.StreamPart{Type: provider.PartResponseMeta, Usage: &provider.Usage{Raw: json.RawMessage(`{"sequence":1}`)}},
			provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{Raw: json.RawMessage(`{"sequence":2}`)}},
		)

		usage, ok := aggregate.Usage()
		require.True(t, ok)
		assert.JSONEq(t, `{"sequence":2}`, string(usage.Raw))
	})
}
