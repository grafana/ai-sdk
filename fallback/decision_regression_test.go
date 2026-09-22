package fallback

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttemptDecision_CancellationDuringDecider(t *testing.T) {
	for _, mode := range []string{"unary", "stream"} {
		for _, cancellation := range []string{"cancel", "deadline"} {
			for _, eligible := range []bool{false, true} {
				name := mode + "/" + cancellation + "/deny"
				if eligible {
					name = mode + "/" + cancellation + "/allow"
				}
				t.Run(name, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
						defer cancel()
						want := error(context.Canceled)
						if cancellation == "deadline" {
							want = context.DeadlineExceeded
						}
						failure := errors.New("setup failed")
						calls, decisions := 0, 0
						var attempts []Attempt
						var child context.Context
						producerDone := make(chan struct{})
						candidate := &mockModel{
							doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
								calls++
								return nil, failure
							},
							doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
								calls++
								child = ctx
								parts := make(chan provider.StreamPart)
								go func() {
									defer close(producerDone)
									defer close(parts)
									<-ctx.Done()
									parts <- provider.StreamPart{Type: provider.PartError}
								}()
								return &provider.StreamResult{Stream: parts}, failure
							},
						}
						model := mustNew(t, candidate, candidate).WithDecider(func(err error) bool {
							decisions++
							assert.ErrorIs(t, err, failure)
							assert.NoError(t, ctx.Err())
							if cancellation == "cancel" {
								cancel()
							} else {
								<-ctx.Done()
							}
							return eligible
						}).WithAttemptObserver(func(_ context.Context, attempt Attempt) {
							attempts = append(attempts, attempt)
						})
						err := invokeFailedDecision(t, model, ctx, mode)
						require.ErrorIs(t, err, want)
						assert.Equal(t, 1, calls)
						assert.Equal(t, 1, decisions)
						require.Len(t, attempts, 1)
						assert.Equal(t, AttemptCanceled, attempts[0].Outcome)
						assert.ErrorIs(t, attempts[0].Err, want)
						assert.False(t, attempts[0].WillFallback)
						assert.False(t, attempts[0].FinishedAt.Before(attempts[0].StartedAt))
						if mode == "stream" {
							assert.ErrorIs(t, child.Err(), want)
							select {
							case <-producerDone:
							case <-time.After(time.Second):
								t.Fatal("canceled error result was not drained")
							}
						}
					})
				})
			}
		}
	}
}

func TestAttemptDecision_ObserverCancellationPreservesIntent(t *testing.T) {
	for _, mode := range []string{"unary", "stream"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("setup failed")
			calls, decisions := 0, 0
			var attempts []Attempt
			candidate := &mockModel{
				doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					calls++
					return nil, failure
				},
				doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					calls++
					return nil, failure
				},
			}
			model := mustNew(t, candidate, candidate).WithDecider(func(error) bool {
				decisions++
				return true
			}).WithAttemptObserver(func(_ context.Context, attempt Attempt) {
				attempts = append(attempts, attempt)
				cancel()
			})
			err := invokeFailedDecision(t, model, ctx, mode)
			require.ErrorIs(t, err, context.Canceled)
			assert.ErrorIs(t, err, failure)
			assert.Equal(t, 1, calls)
			assert.Equal(t, 1, decisions)
			require.Len(t, attempts, 1)
			assert.Equal(t, AttemptFailed, attempts[0].Outcome)
			assert.ErrorIs(t, attempts[0].Err, failure)
			assert.True(t, attempts[0].WillFallback)
		})
	}
}

func invokeFailedDecision(t *testing.T, model *Model, ctx context.Context, mode string) error {
	t.Helper()
	if mode == "unary" {
		result, err := model.DoGenerate(ctx, provider.CallOptions{})
		assert.Nil(t, result)
		return err
	}
	result, err := model.DoStream(ctx, provider.CallOptions{})
	assert.Nil(t, result)
	return err
}
