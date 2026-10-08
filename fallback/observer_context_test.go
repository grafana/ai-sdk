package fallback

import (
	"context"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithAttemptObserver_IndependentDelivery(t *testing.T) {
	for _, mode := range []string{"unary", "stream"} {
		for _, panicIn := range []string{"neither", "request", "model"} {
			t.Run(mode+"/"+panicIn, func(t *testing.T) {
				model, failure := observationModel(t)
				var requestAttempts, modelAttempts []Attempt
				var order []string
				model.WithAttemptObserver(func(_ context.Context, attempt Attempt) {
					modelAttempts = append(modelAttempts, attempt)
					order = append(order, "model")
					if panicIn == "model" {
						panic("operator observer failed")
					}
				})
				ctx := WithAttemptObserver(context.Background(), func(_ context.Context, attempt Attempt) {
					requestAttempts = append(requestAttempts, attempt)
					order = append(order, "request")
					if panicIn == "request" {
						panic("request observer failed")
					}
				})
				invokeObservedSuccess(t, model, ctx, mode)
				require.Len(t, requestAttempts, 2)
				assert.Equal(t, requestAttempts, modelAttempts)
				assert.Equal(t, []string{"request", "model", "request", "model"}, order)
				assert.Same(t, failure, requestAttempts[0].SourceErr)
				assert.Same(t, failure, requestAttempts[0].Err)
				assert.Equal(t, AttemptFailed, requestAttempts[0].Outcome)
				assert.True(t, requestAttempts[0].WillFallback)
				assert.Equal(t, AttemptSelected, requestAttempts[1].Outcome)
				assert.Nil(t, requestAttempts[1].SourceErr)
			})
		}
	}
}

func TestWithAttemptObserver_Inheritance(t *testing.T) {
	for _, mode := range []string{"unary", "stream"} {
		for _, disabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/disabled=%t", mode, disabled), func(t *testing.T) {
				model, _ := observationModel(t)
				parentCalls, childCalls, modelCalls := 0, 0, 0
				model.WithAttemptObserver(func(context.Context, Attempt) { modelCalls++ })
				parent := WithAttemptObserver(context.Background(), func(context.Context, Attempt) { parentCalls++ })
				var childObserver AttemptObserver
				if !disabled {
					childObserver = func(context.Context, Attempt) { childCalls++ }
				}
				child := WithAttemptObserver(parent, childObserver)
				invokeObservedSuccess(t, model, child, mode)
				assert.Zero(t, parentCalls)
				wantChildCalls := 2
				if disabled {
					wantChildCalls = 0
				}
				assert.Equal(t, wantChildCalls, childCalls)
				invokeObservedSuccess(t, model, parent, mode)
				assert.Equal(t, 2, parentCalls)
				assert.Equal(t, 4, modelCalls)
			})
		}
	}
}

func TestWithAttemptObserver_SharedModel(t *testing.T) {
	model, failure := observationModel(t)
	type requestKey struct{}
	for i := range 24 {
		t.Run(fmt.Sprintf("request-%d", i), func(t *testing.T) {
			t.Parallel()
			ctx := context.WithValue(context.Background(), requestKey{}, i)
			var attempts []Attempt
			ctx = WithAttemptObserver(ctx, func(observed context.Context, attempt Attempt) {
				assert.Equal(t, i, observed.Value(requestKey{}))
				attempts = append(attempts, attempt)
			})
			mode := "unary"
			if i%2 != 0 {
				mode = "stream"
			}
			invokeObservedSuccess(t, model, ctx, mode)
			require.Len(t, attempts, 2)
			assert.Equal(t, []int{1, 2}, []int{attempts[0].Index, attempts[1].Index})
			assert.Same(t, failure, attempts[0].SourceErr)
			assert.Equal(t, "secondary", attempts[1].Provider)
		})
	}
}

func observationModel(t *testing.T) (*Model, *provider.APICallError) {
	t.Helper()
	failure := provider.NewAPICallError(provider.APICallErrorOptions{Message: "overloaded", StatusCode: 503})
	primary := &mockModel{
		providerName: "primary",
		modelID:      "first",
		doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, failure
		},
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return nil, failure
		},
	}
	secondary := &mockModel{
		providerName: "secondary",
		modelID:      "second",
		doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{}, nil
		},
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			parts := make(chan provider.StreamPart, 1)
			parts <- provider.StreamPart{Type: provider.PartFinish}
			close(parts)
			return &provider.StreamResult{Stream: parts}, nil
		},
	}
	return mustNew(t, primary, secondary), failure
}

func invokeObservedSuccess(t *testing.T, model *Model, ctx context.Context, mode string) {
	t.Helper()
	if mode == "unary" {
		result, err := model.DoGenerate(ctx, provider.CallOptions{})
		require.NoError(t, err)
		require.NotNil(t, result)
		return
	}
	result, err := model.DoStream(ctx, provider.CallOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	require.Len(t, parts, 1)
	assert.Equal(t, provider.PartFinish, parts[0].Type)
}
