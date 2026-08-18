package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withRetryTiming(initDelayMs, backoff float64) Option {
	return sharedOption{fn: func(c *baseConfig) {
		c.retryInitDelay = initDelayMs
		c.retryBackoff = backoff
	}}
}

var fastRetryTiming = withRetryTiming(1, 1)

func slowTextStreamParts(text string, delay time.Duration) <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		time.Sleep(delay)
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: text}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)}}}
	}()
	return ch
}

func stallingStreamParts(stallAfter time.Duration) <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "x"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "x"}
		time.Sleep(stallAfter)
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)}}}
	}()
	return ch
}

func TestStreamText_RetryOnTransientError(t *testing.T) {
	var callCount atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			n := callCount.Add(1)
			if n == 1 {
				return nil, retryableAPIError("transient", nil)
			}
			return &provider.StreamResult{Stream: textStreamParts("recovered")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithMaxRetries(2),
		fastRetryTiming,
	)

	for range result.FullStream() {
	}

	assert.Equal(t, "recovered", result.Text())
	assert.NoError(t, result.Err())
	assert.Equal(t, int32(2), callCount.Load())
}

func TestStreamText_RetryablePartErrorIsNotRetried(t *testing.T) {
	var callCount atomic.Int32
	retryable := true
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callCount.Add(1)
			ch := make(chan provider.StreamPart, 3)
			ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "partial"}
			ch <- provider.StreamPart{
				Type: provider.PartError,
				APICallError: provider.NewAPICallError(provider.APICallErrorOptions{
					Message:     "transient stream failure",
					IsRetryable: &retryable,
				}),
			}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithMaxRetries(2),
		fastRetryTiming,
	)

	for range result.FullStream() {
	}

	assert.Equal(t, "partial", result.Text())
	assert.Equal(t, int32(1), callCount.Load())
	var apiErr *provider.APICallError
	require.ErrorAs(t, result.Err(), &apiErr)
	assert.True(t, apiErr.IsRetryable)
}

func TestStreamText_RetryExhausted(t *testing.T) {
	var callCount atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callCount.Add(1)
			return nil, retryableAPIError("always fails", nil)
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithMaxRetries(1),
		fastRetryTiming,
	)

	for range result.FullStream() {
	}

	err := result.Err()
	require.Error(t, err)
	assert.Equal(t, int32(2), callCount.Load())

	var retryErr *RetryError
	require.ErrorAs(t, err, &retryErr)
	assert.Equal(t, RetryMaxRetriesExceeded, retryErr.Reason)
}

func TestStreamText_RetryDisabledPassesThrough(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return nil, retryableAPIError("transient", nil)
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithMaxRetries(0),
	)

	for range result.FullStream() {
	}

	err := result.Err()
	require.Error(t, err)

	var apiErr *provider.APICallError
	require.ErrorAs(t, err, &apiErr)

	var retryErr *RetryError
	assert.False(t, errors.As(err, &retryErr))
}

func TestStreamText_NonRetryableErrorNotRetried(t *testing.T) {
	var callCount atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callCount.Add(1)
			return nil, errors.New("permanent")
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithMaxRetries(2),
		fastRetryTiming,
	)

	for range result.FullStream() {
	}

	err := result.Err()
	require.Error(t, err)
	assert.Equal(t, int32(1), callCount.Load())
	assert.Equal(t, "permanent", err.Error())
}

func TestStreamText_TotalTimeout(t *testing.T) {
	model := &mockModel{
		streamFunc: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: slowTextStreamParts("slow", 500*time.Millisecond)}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTimeout(TimeoutConfig{Total: 50 * time.Millisecond}),
		WithMaxRetries(0),
	)

	for range result.FullStream() {
	}

	assert.Equal(t, "", result.Text())
}

func TestStreamText_StepTimeout(t *testing.T) {
	model := &mockModel{
		streamFunc: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			time.Sleep(200 * time.Millisecond)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTimeout(TimeoutConfig{Step: 50 * time.Millisecond}),
		WithMaxRetries(0),
	)

	for range result.FullStream() {
	}

	err := result.Err()
	require.Error(t, err)
}

func TestStreamText_ChunkTimeout(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: stallingStreamParts(500 * time.Millisecond)}, nil
		},
	}

	var gotError bool
	var textParts []string
	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTimeout(TimeoutConfig{Chunk: 50 * time.Millisecond}),
		WithMaxRetries(0),
	)

	for part := range result.FullStream() {
		switch p := part.(type) {
		case StreamTextDelta:
			textParts = append(textParts, p.Text)
		case StreamError:
			gotError = true
		}
	}

	assert.Equal(t, []string{"x", "x"}, textParts, "should receive parts before stall")
	assert.False(t, gotError, "chunk timeout causes abort, not explicit error")
}

func TestStreamText_EmptyDeltasDoNotCountAsOutput(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart, 3)
			stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
			stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1"}
			close(stream)
			return &provider.StreamResult{Stream: stream}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithMaxRetries(0),
	)
	for range result.FullStream() {
	}

	require.Error(t, result.Err())
	assert.ErrorIs(t, result.Err(), ErrNoOutputGenerated)
}

func TestStreamText_FirstChunkTimeoutIgnoresNonSemanticParts(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart, 16)
			go func() {
				defer close(stream)
				stream <- provider.StreamPart{Type: provider.PartStreamStart}
				stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
				for range 5 {
					time.Sleep(20 * time.Millisecond)
					stream <- provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(`{}`)}
				}
				stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "late"}
			}()
			return &provider.StreamResult{Stream: stream}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTimeout(TimeoutConfig{FirstChunk: 50 * time.Millisecond}),
		WithMaxRetries(0),
	)
	for range result.FullStream() {
	}

	assert.Empty(t, result.Text())
}

func TestStreamText_FirstChunkTimeoutRearmsForEachStep(t *testing.T) {
	var callCount int
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callCount++
			if callCount == 1 {
				stream := make(chan provider.StreamPart, 2)
				stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "lookup", Input: `{}`}
				stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
				close(stream)
				return &provider.StreamResult{Stream: stream}, nil
			}
			stream := make(chan provider.StreamPart, 1)
			go func() {
				defer close(stream)
				time.Sleep(100 * time.Millisecond)
				stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-2", Delta: "late"}
			}()
			return &provider.StreamResult{Stream: stream}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{"lookup": {
			InputSchema: testMustSchema(t, `{"type":"object"}`),
			Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
				return json.RawMessage(`{"ok":true}`), nil
			},
		}}),
		WithStopWhen(StepCountIs(2)),
		WithTimeout(TimeoutConfig{FirstChunk: 50 * time.Millisecond}),
		WithMaxRetries(0),
	)
	for range result.FullStream() {
	}

	assert.Equal(t, 2, callCount)
	assert.Empty(t, result.Text())
}

func TestStreamText_FirstChunkTimeoutStopsAfterSemanticOutput(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart, 8)
			go func() {
				defer close(stream)
				stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
				stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "ready"}
				time.Sleep(100 * time.Millisecond)
				stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
				stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
			}()
			return &provider.StreamResult{Stream: stream}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTimeout(TimeoutConfig{FirstChunk: 50 * time.Millisecond}),
		WithMaxRetries(0),
	)
	for range result.FullStream() {
	}

	assert.Equal(t, "ready", result.Text())
	assert.NoError(t, result.Err())
}

func TestStreamText_ChunkTimeoutIgnoresNonSemanticParts(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart, 16)
			go func() {
				defer close(stream)
				stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
				stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "x"}
				for range 5 {
					time.Sleep(20 * time.Millisecond)
					stream <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "response-1"}
				}
				stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "late"}
			}()
			return &provider.StreamResult{Stream: stream}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTimeout(TimeoutConfig{Chunk: 50 * time.Millisecond}),
		WithMaxRetries(0),
	)
	var deltas []string
	for part := range result.FullStream() {
		if delta, ok := part.(StreamTextDelta); ok {
			deltas = append(deltas, delta.Text)
		}
	}

	assert.Equal(t, []string{"x"}, deltas)
}

func TestStreamText_TimeoutCancelsRetry(t *testing.T) {
	var callCount atomic.Int32
	model := &mockModel{
		streamFunc: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callCount.Add(1)
			return nil, retryableAPIError("transient", nil)
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithMaxRetries(100),
		WithTimeout(TimeoutConfig{Step: 100 * time.Millisecond}),
		withRetryTiming(30, 1),
	)

	for range result.FullStream() {
	}

	err := result.Err()
	require.Error(t, err)
	calls := callCount.Load()
	assert.Greater(t, calls, int32(1), "should have retried at least once")
	assert.Less(t, calls, int32(100), "step timeout should have prevented all retries")
}

func TestStreamText_PerStepRetryIndependence(t *testing.T) {
	var callCount atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			n := callCount.Add(1)
			switch n {
			case 1:
				return nil, retryableAPIError("step1-fail", nil)
			case 2:
				return &provider.StreamResult{Stream: toolCallStreamParts("echo", `{"text":"hi"}`)}, nil
			case 3:
				return nil, retryableAPIError("step2-fail", nil)
			default:
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{
			"echo": Tool{
				Description: "Echo",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, input json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return input, nil
				},
			},
		}),
		WithStopWhen(StepCountIs(3)),
		WithMaxRetries(1),
		fastRetryTiming,
	)

	for range result.FullStream() {
	}

	assert.Equal(t, "done", result.Text())
	assert.NoError(t, result.Err())
	assert.Equal(t, int32(4), callCount.Load(), "2 steps with 1 retry each = 4 calls")
}

func TestGenerateText_RetryAndTimeout(t *testing.T) {
	var callCount atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			n := callCount.Add(1)
			if n == 1 {
				return nil, retryableAPIError("transient", nil)
			}
			return &provider.StreamResult{Stream: textStreamParts("recovered")}, nil
		},
	}

	gen, err := GenerateText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithMaxRetries(2),
		WithTimeout(TimeoutConfig{Total: 5 * time.Second}),
		fastRetryTiming,
	)

	require.NoError(t, err)
	assert.Equal(t, "recovered", gen.Text)
	assert.Equal(t, int32(2), callCount.Load())
}

func TestStreamText_NegativeMaxRetriesTreatedAsZero(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return nil, retryableAPIError("transient", nil)
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithMaxRetries(-5),
	)

	for range result.FullStream() {
	}

	err := result.Err()
	require.Error(t, err)

	var apiErr *provider.APICallError
	require.ErrorAs(t, err, &apiErr, "negative maxRetries should behave as 0 (no retry, unwrapped error)")

	var retryErr *RetryError
	assert.False(t, errors.As(err, &retryErr), "should not be wrapped in RetryError")
}

func TestStreamText_StepTimeoutResetsBetweenSteps(t *testing.T) {
	var callCount atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			n := callCount.Add(1)
			time.Sleep(30 * time.Millisecond)
			if n == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("echo", `{"v":1}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{
			"echo": Tool{
				Description: "Echo",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, input json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return input, nil
				},
			},
		}),
		WithStopWhen(StepCountIs(3)),
		WithTimeout(TimeoutConfig{Step: 100 * time.Millisecond}),
		WithMaxRetries(0),
	)

	for range result.FullStream() {
	}

	assert.Equal(t, "done", result.Text(), "both steps should complete within their individual step timeouts")
	assert.NoError(t, result.Err())
	assert.Equal(t, int32(2), callCount.Load())
}

func TestStreamText_StepTimeoutAbortsEntireOperation(t *testing.T) {
	var callCount atomic.Int32
	model := &mockModel{
		streamFunc: func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			n := callCount.Add(1)
			if n == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("echo", `{"v":1}`)}, nil
			}
			time.Sleep(200 * time.Millisecond)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return &provider.StreamResult{Stream: textStreamParts("should not reach")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{
			"echo": Tool{
				Description: "Echo",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, input json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return input, nil
				},
			},
		}),
		WithStopWhen(StepCountIs(3)),
		WithTimeout(TimeoutConfig{Step: 50 * time.Millisecond}),
		WithMaxRetries(0),
	)

	for range result.FullStream() {
	}

	err := result.Err()
	require.Error(t, err, "step timeout in step 2 should abort the entire operation")
	assert.Equal(t, int32(2), callCount.Load())
}

func TestStreamText_TotalTimeoutAcrossMultipleSteps(t *testing.T) {
	var callCount atomic.Int32
	model := &mockModel{
		streamFunc: func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			n := callCount.Add(1)
			time.Sleep(40 * time.Millisecond)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if n == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("echo", `{"v":1}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("should not reach")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{
			"echo": Tool{
				Description: "Echo",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, input json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return input, nil
				},
			},
		}),
		WithStopWhen(StepCountIs(3)),
		WithTimeout(TimeoutConfig{Total: 60 * time.Millisecond}),
		WithMaxRetries(0),
	)

	for range result.FullStream() {
	}

	assert.NotEqual(t, "should not reach", result.Text(), "total timeout should abort before step 2 completes")
}

func TestGenerateText_FirstChunkTimeoutIgnored(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: slowTextStreamParts("slow", 100*time.Millisecond)}, nil
		},
	}

	gen, err := GenerateText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTimeout(TimeoutConfig{FirstChunk: 20 * time.Millisecond}),
		WithMaxRetries(0),
	)

	require.NoError(t, err, "first chunk timeout should be ignored for GenerateText")
	assert.Equal(t, "slow", gen.Text)
	assert.Equal(t, []provider.Warning{{
		Type:    provider.WarnUnsupported,
		Feature: "timeout.firstChunkMs",
		Details: "The firstChunkMs timeout is only supported by streaming functions.",
	}}, gen.Warnings)
}

func TestGenerateText_ChunkTimeoutIgnored(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: stallingStreamParts(200 * time.Millisecond)}, nil
		},
	}

	gen, err := GenerateText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTimeout(TimeoutConfig{Chunk: 50 * time.Millisecond}),
		WithMaxRetries(0),
	)

	require.NoError(t, err, "chunk timeout should be ignored for GenerateText")
	assert.Equal(t, "xx", gen.Text)
	assert.Equal(t, []provider.Warning{{
		Type:    provider.WarnUnsupported,
		Feature: "timeout.chunkMs",
		Details: "The chunkMs timeout is only supported by streaming functions.",
	}}, gen.Warnings)
}

func TestStreamText_DefaultMaxRetries(t *testing.T) {
	var callCount atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			n := callCount.Add(1)
			if n <= 2 {
				return nil, retryableAPIError("transient", nil)
			}
			return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		fastRetryTiming,
	)

	for range result.FullStream() {
	}

	assert.Equal(t, "ok", result.Text())
	assert.NoError(t, result.Err())
	assert.Equal(t, int32(3), callCount.Load(), "default maxRetries=2 means 3 total attempts")
}
