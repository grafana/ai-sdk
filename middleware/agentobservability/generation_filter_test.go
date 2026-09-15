package agentobservability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordingMiddleware_GenerationFilterAppliesToUnaryAndStream(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			env := testkit.NewEnv(t)
			calls := 0
			opts := RecordingOptions{
				ClientResolver: func(context.Context) *agento11y.Client { return env.Client },
				GenerationFilter: func(input GenerationFilterInput) agento11y.Generation {
					calls++
					assert.Equal(t, provider.FinishReasonStop, input.FinishReason.Unified)
					generation := input.Generation
					generation.Metadata = map[string]any{"filtered": mode}
					return generation
				},
			}
			model := &mockLanguageModel{provider_: "grafana", modelID: "grafana/assistant"}
			if mode == "generate" {
				_, err := generateWith(t, model, opts)
				require.NoError(t, err)
			} else {
				result, err := streamWith(t, model, opts)
				require.NoError(t, err)
				for range result.Stream {
				}
				require.Eventually(t, func() bool { return calls == 1 }, time.Second, time.Millisecond)
			}

			assert.Equal(t, 1, calls)
			require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
			require.NoError(t, env.Client.Shutdown(context.Background()))
			generation := env.SingleGenerationJSON(t)
			metadata, ok := generation["metadata"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, mode, metadata["filtered"])
		})
	}
}

func TestRecordingMiddleware_GenerationFilterAppliesToNilStream(t *testing.T) {
	env := testkit.NewEnv(t)
	calls := 0
	model := &mockLanguageModel{
		provider_: "grafana",
		modelID:   "grafana/assistant",
		doStream:  func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, nil },
	}
	result, err := streamWith(t, model, RecordingOptions{
		ClientResolver: func(context.Context) *agento11y.Client { return env.Client },
		GenerationFilter: func(input GenerationFilterInput) agento11y.Generation {
			calls++
			return input.Generation
		},
	})
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 1, calls)
}

func TestFilterGeneration_PanicFailsClosed(t *testing.T) {
	start := agento11y.GenerationStart{Model: agento11y.ModelRef{Provider: "grafana", Name: "grafana/assistant"}}
	filtered := filterGeneration(agento11y.Generation{
		Model:    agento11y.ModelRef{Provider: "private", Name: "private"},
		Metadata: map[string]any{"secret": "must-not-survive"},
	}, start, provider.FinishReason{}, func(GenerationFilterInput) agento11y.Generation {
		panic(errors.New("private panic"))
	})

	assert.Equal(t, start.Model, filtered.Model)
	assert.Empty(t, filtered.Metadata)
}
