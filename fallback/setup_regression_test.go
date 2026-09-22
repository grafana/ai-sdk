package fallback

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoStream_SetupPanicContained(t *testing.T) {
	if os.Getenv("AI_SDK_FALLBACK_PANIC_PROBE") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDoStream_SetupPanicContained$", "-test.count=1")
		command.Env = append(os.Environ(), "AI_SDK_FALLBACK_PANIC_PROBE=1")
		output, err := command.CombinedOutput()
		require.NoError(t, err, "setup panic must not crash or hang the process: %s", output)
		return
	}
	for _, allow := range []bool{false, true} {
		var attempts []Attempt
		var candidateCtx context.Context
		decisions := 0
		model := mustNew(t, &mockModel{doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			candidateCtx = ctx
			panic("private provider panic value")
		}}, &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			parts := make(chan provider.StreamPart, 1)
			parts <- provider.StreamPart{Type: provider.PartFinish}
			close(parts)
			return &provider.StreamResult{Stream: parts}, nil
		}}).WithDecider(func(err error) bool {
			decisions++
			assert.EqualError(t, err, "fallback: candidate stream setup panicked")
			return allow
		}).WithAttemptObserver(func(_ context.Context, attempt Attempt) { attempts = append(attempts, attempt) })
		result, err := model.DoStream(context.Background(), provider.CallOptions{})
		if allow {
			require.NoError(t, err)
			for range result.Stream {
			}
			require.Len(t, attempts, 2)
			assert.Equal(t, AttemptSelected, attempts[1].Outcome)
		} else {
			require.EqualError(t, err, "fallback: candidate stream setup panicked")
			require.Len(t, attempts, 1)
		}
		assert.Equal(t, 1, decisions)
		assert.Equal(t, AttemptFailed, attempts[0].Outcome)
		assert.Equal(t, allow, attempts[0].WillFallback)
		assert.EqualError(t, attempts[0].Err, "fallback: candidate stream setup panicked")
		assert.ErrorIs(t, candidateCtx.Err(), context.Canceled)
	}
}

func TestPrecommitFailure_DeciderIncludesFinalCandidate(t *testing.T) {
	failure := errors.New("setup")
	for _, mode := range []string{"unary", "stream setup", "stream nil", "stream EOF"} {
		for _, count := range []int{1, 2} {
			t.Run(mode+string(rune('0'+count)), func(t *testing.T) {
				var candidates []provider.LanguageModel
				for range count {
					candidates = append(candidates, &mockModel{
						doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, failure },
						doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
							switch mode {
							case "stream nil":
								return nil, nil
							case "stream EOF":
								return &provider.StreamResult{Stream: closedStream()}, nil
							default:
								return nil, failure
							}
						},
					})
				}
				decisions := 0
				var attempts []Attempt
				model := mustNew(t, candidates...).WithDecider(func(error) bool { decisions++; return true }).WithAttemptObserver(func(_ context.Context, a Attempt) { attempts = append(attempts, a) })
				var err error
				if mode == "unary" {
					_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
				} else {
					_, err = model.DoStream(context.Background(), provider.CallOptions{})
				}
				require.Error(t, err)
				assert.Equal(t, count, decisions)
				require.Len(t, attempts, count)
				for i, attempt := range attempts {
					assert.Equal(t, AttemptFailed, attempt.Outcome)
					assert.Equal(t, i+1 < count, attempt.WillFallback)
				}
			})
		}
	}
}
