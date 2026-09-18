package fallback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoStream_CommitmentBoundary(t *testing.T) {
	for _, first := range []provider.StreamPart{
		{Type: provider.PartError},
		{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "retryable", StatusCode: 503})},
		{Type: provider.StreamPartType("unrecognized")},
		{Type: provider.PartTextDelta, Delta: ""},
	} {
		t.Run(string(first.Type)+first.Delta, func(t *testing.T) {
			parts := []provider.StreamPart{first, {Type: provider.PartError}, {Type: provider.PartTextDelta, Delta: "after"}}
			ch := make(chan provider.StreamPart, len(parts))
			for _, part := range parts {
				ch <- part
			}
			close(ch)
			calls, decisions := 0, 0
			m := mustNew(t, &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: ch}, nil
			}}, &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				return nil, errors.New("must not run")
			}}).WithDecider(func(error) bool { decisions++; return true })
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			var got []provider.StreamPart
			for part := range result.Stream {
				got = append(got, part)
			}
			assert.Equal(t, parts, got)
			assert.Zero(t, calls)
			assert.Zero(t, decisions)
		})
	}
}

func TestDoStream_PrematureEOFSelectsNext(t *testing.T) {
	empty := make(chan provider.StreamPart)
	close(empty)
	ready := make(chan provider.StreamPart, 1)
	ready <- provider.StreamPart{Type: provider.PartFinish}
	close(ready)
	var abandoned context.Context
	calls := 0
	m := mustNew(t, &mockModel{doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		abandoned = ctx
		return &provider.StreamResult{Stream: empty}, nil
	}}, &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		calls++
		return &provider.StreamResult{Stream: ready}, nil
	}})
	result, err := m.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.ErrorIs(t, abandoned.Err(), context.Canceled)
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	require.Len(t, parts, 1)
}

func TestDoStream_CanceledBlockedConsumer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := make(chan provider.StreamPart, 256)
	for range cap(input) {
		input <- provider.StreamPart{Type: provider.PartTextDelta}
	}
	var candidateContext context.Context
	m := mustNew(t, &mockModel{doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		candidateContext = ctx
		return &provider.StreamResult{Stream: input}, nil
	}})
	result, err := m.DoStream(ctx, provider.CallOptions{})
	require.NoError(t, err)
	cancel()
	assert.Eventually(t, func() bool { return candidateContext.Err() != nil }, time.Second, time.Millisecond)
	done := make(chan struct{})
	go func() {
		for range result.Stream {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fallback relay retained after cancellation")
	}
}

func TestDoStream_PrecommitDecisions(t *testing.T) {
	failure := errors.New("setup failed")
	for _, tc := range []struct {
		name   string
		result *provider.StreamResult
		err    error
		want   error
	}{
		{name: "setup", err: failure, want: failure},
		{name: "nil result", want: ErrInvalidResult},
		{name: "nil channel", result: &provider.StreamResult{}, want: ErrInvalidResult},
		{name: "premature EOF", result: &provider.StreamResult{Stream: closedStream()}, want: ErrPrematureStreamEnd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, allow := range []bool{false, true} {
				var attempts []Attempt
				var candidateContext context.Context
				primary := &mockModel{providerName: "first", modelID: "one", doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
					candidateContext = ctx
					return tc.result, tc.err
				}}
				secondary := &mockModel{providerName: "second", modelID: "two", doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					ch := make(chan provider.StreamPart, 1)
					ch <- provider.StreamPart{Type: provider.PartFinish}
					close(ch)
					return &provider.StreamResult{Stream: ch}, nil
				}}
				m := mustNew(t, primary, secondary).WithDecider(func(err error) bool { assert.ErrorIs(t, err, tc.want); return allow }).WithAttemptObserver(func(_ context.Context, attempt Attempt) { attempts = append(attempts, attempt) })
				result, err := m.DoStream(context.Background(), provider.CallOptions{})
				if allow {
					require.NoError(t, err)
					for range result.Stream {
					}
					require.Len(t, attempts, 2)
					assert.Equal(t, AttemptSelected, attempts[1].Outcome)
					assert.False(t, attempts[1].WillFallback)
				} else {
					require.ErrorIs(t, err, tc.want)
					require.Len(t, attempts, 1)
				}
				assert.ErrorIs(t, candidateContext.Err(), context.Canceled)
				assert.Equal(t, AttemptFailed, attempts[0].Outcome)
				assert.Equal(t, allow, attempts[0].WillFallback)
				assert.ErrorIs(t, attempts[0].Err, tc.want)
				assert.Equal(t, 1, attempts[0].Index)
				assert.Equal(t, "first", attempts[0].Provider)
				assert.False(t, attempts[0].FinishedAt.Before(attempts[0].StartedAt))
			}
		})
	}
}

func closedStream() <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart)
	close(ch)
	return ch
}

func TestDoStream_CancellationOwnership(t *testing.T) {
	for _, duringSetup := range []bool{false, true} {
		t.Run(map[bool]string{false: "peek", true: "setup"}[duringSetup], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{})
			release := make(chan struct{})
			producerDone := make(chan struct{})
			parts := make(chan provider.StreamPart)
			var attempts []Attempt
			m := mustNew(t, &mockModel{doStream: func(candidateCtx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				close(started)
				if duringSetup {
					<-release
				}
				go func() {
					defer close(producerDone)
					defer close(parts)
					<-candidateCtx.Done()
					parts <- provider.StreamPart{Type: provider.PartError}
				}()
				return &provider.StreamResult{Stream: parts}, nil
			}}).WithAttemptObserver(func(_ context.Context, attempt Attempt) { attempts = append(attempts, attempt) })
			done := make(chan error, 1)
			go func() { _, err := m.DoStream(ctx, provider.CallOptions{}); done <- err }()
			<-started
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				t.Fatal("setup or peek did not cancel")
			}
			close(release)
			select {
			case <-producerDone:
			case <-time.After(time.Second):
				t.Fatal("late producer was not drained")
			}
			require.Len(t, attempts, 1)
			assert.Equal(t, AttemptCanceled, attempts[0].Outcome)
			assert.False(t, attempts[0].WillFallback)
		})
	}
}

func TestDrainStream_Bounds(t *testing.T) {
	t.Run("silent", func(t *testing.T) {
		started := time.Now()
		drainStream(make(chan provider.StreamPart))
		assert.Less(t, time.Since(started), time.Second)
	})
	t.Run("continuously ready", func(t *testing.T) {
		parts := make(chan provider.StreamPart, cleanupPartBudget+10)
		for range cap(parts) {
			parts <- provider.StreamPart{}
		}
		drainStream(parts)
		assert.Len(t, parts, 10)
	})
}

func TestAttemptObserver_PanicIsIsolated(t *testing.T) {
	m := mustNew(t, &mockModel{
		doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{}, nil
		},
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 1)
			ch <- provider.StreamPart{Type: provider.PartError}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}).WithAttemptObserver(func(context.Context, Attempt) { panic("observer failure") })
	_, err := m.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	result, err := m.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	for range result.Stream {
	}
}

func TestDoStream_ExhaustionAndMetadata(t *testing.T) {
	firstErr, secondErr := errors.New("first"), errors.New("second")
	var attempts []Attempt
	m := mustNew(t,
		&mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, firstErr }},
		&mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, secondErr }},
	).WithAttemptObserver(func(_ context.Context, attempt Attempt) { attempts = append(attempts, attempt) })
	_, err := m.DoStream(context.Background(), provider.CallOptions{})
	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, secondErr)
	require.Len(t, attempts, 2)
	assert.True(t, attempts[0].WillFallback)
	assert.False(t, attempts[1].WillFallback)
	assert.Equal(t, AttemptFailed, attempts[1].Outcome)

	input := make(chan provider.StreamPart, 1)
	input <- provider.StreamPart{Type: provider.PartResponseMeta, ModelID: "served", Provider: "physical"}
	close(input)
	original := &provider.StreamResult{Stream: input}
	m = mustNew(t, &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return original, nil }})
	got, err := m.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assert.Equal(t, original.Request, got.Request)
	assert.Equal(t, original.Response, got.Response)
	part := <-got.Stream
	assert.Equal(t, "served", part.ModelID)
	assert.Equal(t, "physical", part.Provider)
	for range got.Stream {
	}
}

func TestDoGenerate_CanceledAndInvalidDecisions(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		var attempts []Attempt
		m := mustNew(t, &mockModel{doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			if canceled {
				cancel()
			}
			return nil, nil
		}}).WithAttemptObserver(func(_ context.Context, attempt Attempt) { attempts = append(attempts, attempt) })
		_, err := m.DoGenerate(ctx, provider.CallOptions{})
		cancel()
		require.Len(t, attempts, 1)
		assert.False(t, attempts[0].WillFallback)
		if canceled {
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, AttemptCanceled, attempts[0].Outcome)
		} else {
			require.ErrorIs(t, err, ErrInvalidResult)
			assert.Equal(t, AttemptFailed, attempts[0].Outcome)
		}
	}
}
