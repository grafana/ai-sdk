package fallback

import (
	"context"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rejectionFor(name string) error {
	return fmt.Errorf("%s: unsupported tool content part type: custom: %w", name, provider.ErrUnsupportedFunctionality)
}

type rejectingCandidate struct {
	*mockModel
	generateCalls *int
	streamCalls   *int
}

func newRejecting(name string) rejectingCandidate {
	generateCalls, streamCalls := new(int), new(int)
	err := rejectionFor(name)
	return rejectingCandidate{
		generateCalls: generateCalls,
		streamCalls:   streamCalls,
		mockModel: &mockModel{
			providerName: name,
			modelID:      name + "-model",
			doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				*generateCalls++
				return nil, err
			},
			doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				*streamCalls++
				return nil, err
			},
		},
	}
}

func succeedingCandidate() *mockModel {
	return &mockModel{
		providerName: "ok",
		modelID:      "ok-model",
		doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
		},
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			parts := make(chan provider.StreamPart, 1)
			parts <- provider.StreamPart{Type: provider.PartFinish}
			close(parts)
			return &provider.StreamResult{Stream: parts}, nil
		},
	}
}

func TestUnsupportedFunctionality_Fallback(t *testing.T) {
	modes := []struct {
		name  string
		run   func(*Model) error
		calls func(rejectingCandidate) int
	}{
		{
			name: "unary",
			run: func(m *Model) error {
				_, err := m.DoGenerate(context.Background(), provider.CallOptions{})
				return err
			},
			calls: func(c rejectingCandidate) int { return *c.generateCalls },
		},
		{
			name: "stream setup",
			run: func(m *Model) error {
				result, err := m.DoStream(context.Background(), provider.CallOptions{})
				if err == nil {
					for range result.Stream {
					}
				}
				return err
			},
			calls: func(c rejectingCandidate) int { return *c.streamCalls },
		},
	}

	for _, mode := range modes {
		t.Run(mode.name+"/default decider advances to a succeeding candidate", func(t *testing.T) {
			rejecting := newRejecting("bedrock")
			model := mustNew(t, rejecting, succeedingCandidate())

			require.NoError(t, mode.run(model))
			assert.Equal(t, 1, mode.calls(rejecting))
		})

		t.Run(mode.name+"/all candidates rejecting keeps every identity", func(t *testing.T) {
			first, second := newRejecting("first"), newRejecting("second")
			model := mustNew(t, first, second)

			err := mode.run(model)

			require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
			assert.Equal(t, 1, mode.calls(first))
			assert.Equal(t, 1, mode.calls(second))
			assert.Contains(t, err.Error(), "first: unsupported tool content part type: custom")
			assert.Contains(t, err.Error(), "second: unsupported tool content part type: custom")
		})

		t.Run(mode.name+"/custom decider returning false stops", func(t *testing.T) {
			rejecting := newRejecting("bedrock")
			next := newRejecting("next")
			model := mustNew(t, rejecting, next).WithDecider(func(error) bool { return false })

			err := mode.run(model)

			require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
			assert.Equal(t, 1, mode.calls(rejecting))
			assert.Zero(t, mode.calls(next))
		})
	}
}
