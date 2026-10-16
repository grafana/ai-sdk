package fallback

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bufferedResult(parts ...provider.StreamPart) *provider.StreamResult {
	ch := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		ch <- part
	}
	close(ch)
	return &provider.StreamResult{Stream: ch}
}

func completedParts() []provider.StreamPart {
	input, output := 50, 5
	return []provider.StreamPart{
		{Type: provider.PartStreamStart},
		{Type: provider.PartResponseMeta, Provider: "anthropic", ModelID: "claude-haiku-5-5"},
		{Type: provider.PartTextStart, ID: "answer"},
		{Type: provider.PartTextDelta, ID: "answer", Delta: `{"status":"ok"}`},
		{Type: provider.PartTextEnd, ID: "answer"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: &input}, OutputTokens: provider.OutputTokenUsage{Total: &output}}},
	}
}

func readParts(result *provider.StreamResult) []provider.StreamPart {
	var parts []provider.StreamPart
	for p := range result.Stream {
		parts = append(parts, p)
	}
	return parts
}

func TestBufferedStream_RecoversWithoutLeakingFailedOutput(t *testing.T) {
	retryable := provider.NewAPICallError(provider.APICallErrorOptions{Message: "unavailable", StatusCode: 503})
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
	}{
		{"leading error", []provider.StreamPart{{Type: provider.PartError, APICallError: retryable}}},
		{"partial error", []provider.StreamPart{{Type: provider.PartStreamStart}, {Type: provider.PartTextStart, ID: "failed"}, {Type: provider.PartTextDelta, ID: "failed", Delta: "discard me"}, {Type: provider.PartError, APICallError: retryable}}},
		{"partial EOF", []provider.StreamPart{{Type: provider.PartTextDelta, ID: "failed", Delta: "discard me"}}},
		{"error after finish", append(completedParts(), provider.StreamPart{Type: provider.PartError, APICallError: retryable})},
		{"empty EOF", nil},
		{"finish error", []provider.StreamPart{{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonError}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts []Attempt
			var abandoned context.Context
			primary := &mockModel{providerName: "bedrock-mantle.responses", modelID: "gpt-6-luna", doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				abandoned = ctx
				return bufferedResult(tc.parts...), nil
			}}
			secondary := &mockModel{providerName: "openai", modelID: "gpt-6-luna", doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, retryable }}
			backup := &mockModel{providerName: "anthropic", modelID: "claude-haiku-5-5", doStream: func(_ context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
				assert.Equal(t, provider.ReasoningMedium, params.Reasoning)
				return bufferedResult(completedParts()...), nil
			}}
			m, err := mustNew(t, primary, secondary, backup).WithAttemptObserver(func(_ context.Context, a Attempt) { attempts = append(attempts, a) }).WithBufferedStream(1<<20, time.Second)
			require.NoError(t, err)
			result, err := m.DoStream(context.Background(), provider.CallOptions{Reasoning: provider.ReasoningMedium})
			require.NoError(t, err)
			assert.Equal(t, completedParts(), readParts(result))
			assert.ErrorIs(t, abandoned.Err(), context.Canceled)
			require.Len(t, attempts, 3)
			assert.Equal(t, AttemptFailed, attempts[0].Outcome)
			assert.True(t, attempts[0].WillFallback)
			assert.Equal(t, "claude-haiku-5-5", attempts[2].ModelID)
			assert.Equal(t, AttemptSelected, attempts[2].Outcome)
		})
	}
}

func TestBufferedStream_BoundsAndPolicy(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    *provider.StreamResult
		maxBytes  int
		want      error
		apiStatus int
	}{
		{"byte limit", bufferedResult(provider.StreamPart{Type: provider.PartTextDelta, Delta: strings.Repeat("x", 256)}), 128, ErrStreamBufferLimit, 0},
		{"part limit", bufferedResult(make([]provider.StreamPart, maxBufferedStreamParts+1)...), 1 << 20, ErrStreamBufferLimit, 0},
		{"bad request", bufferedResult(provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "invalid schema", StatusCode: 400})}), 1 << 20, nil, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			m, err := mustNew(t, &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return tc.result, nil }}, &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				return bufferedResult(completedParts()...), nil
			}}).WithBufferedStream(tc.maxBytes, time.Second)
			require.NoError(t, err)
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.Error(t, err)
			assert.Nil(t, result)
			if tc.want != nil {
				assert.ErrorIs(t, err, tc.want)
			}
			if tc.apiStatus != 0 {
				var apiErr *provider.APICallError
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, tc.apiStatus, apiErr.StatusCode)
			}
			assert.Zero(t, calls)
		})
	}
}

func TestBufferedStream_TimeoutAndCancellation(t *testing.T) {
	for _, setup := range []bool{false, true} {
		t.Run(map[bool]string{true: "setup timeout", false: "midstream timeout"}[setup], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			attemptContexts := make(chan context.Context, 1)
			primary := &mockModel{doStream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				attemptContexts <- ctx
				if setup {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				ch := make(chan provider.StreamPart, 1)
				ch <- provider.StreamPart{Type: provider.PartStreamStart}
				go func() { <-ctx.Done(); close(ch) }()
				return &provider.StreamResult{Stream: ch}, nil
			}}
			backup := &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return bufferedResult(completedParts()...), nil
			}}
			m, err := mustNew(t, primary, backup).WithBufferedStream(1<<20, 20*time.Millisecond)
			require.NoError(t, err)
			result, err := m.DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)
			assert.Equal(t, completedParts(), readParts(result))
			assert.ErrorIs(t, (<-attemptContexts).Err(), context.DeadlineExceeded)
		})
	}
	t.Run("parent cancellation never falls back", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		m, err := mustNew(t, &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			cancel()
			return nil, errors.New("failed")
		}}, &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { calls++; return nil, nil }}).WithBufferedStream(1<<20, time.Second)
		require.NoError(t, err)
		_, err = m.DoStream(ctx, provider.CallOptions{})
		assert.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, calls)
	})
}

func TestBufferedStream_RejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		bytes   int
		timeout time.Duration
	}{{0, time.Second}, {-1, time.Second}, {1, 0}, {1, -time.Second}} {
		_, err := mustNew(t, &mockModel{}).WithBufferedStream(tc.bytes, tc.timeout)
		require.Error(t, err)
	}
}

func TestBufferedStream_ObserversRetainSourceErrorAfterDecisionCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nativeErr := provider.NewAPICallError(provider.APICallErrorOptions{Message: "interrupted", StatusCode: 503})
	var observed []Attempt
	var order []string
	ctx = WithAttemptObserver(ctx, func(_ context.Context, a Attempt) {
		order = append(order, "request")
		observed = append(observed, a)
	})
	model := mustNew(t, &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return bufferedResult(provider.StreamPart{Type: provider.PartTextDelta, Delta: "discarded"}, provider.StreamPart{Type: provider.PartError, APICallError: nativeErr}), nil
	}}).WithDecider(func(err error) bool {
		require.Same(t, nativeErr, err)
		cancel()
		return true
	}).WithAttemptObserver(func(_ context.Context, a Attempt) {
		order = append(order, "model")
		observed = append(observed, a)
	})
	model, err := model.WithBufferedStream(1<<20, time.Second)
	require.NoError(t, err)
	result, err := model.DoStream(ctx, provider.CallOptions{})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
	assert.Equal(t, []string{"request", "model"}, order)
	require.Len(t, observed, 2)
	for _, attempt := range observed {
		assert.ErrorIs(t, attempt.Err, context.Canceled)
		assert.Same(t, nativeErr, attempt.SourceErr)
		assert.Equal(t, AttemptCanceled, attempt.Outcome)
		assert.False(t, attempt.WillFallback)
	}
}
