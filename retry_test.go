package aisdk

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func retryableAPIError(msg string, headers map[string][]string) *provider.APICallError {
	retryable := true
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:         msg,
		StatusCode:      429,
		ResponseHeaders: headers,
		IsRetryable:     &retryable,
	})
}

func nonRetryableAPIError(msg string) *provider.APICallError {
	retryable := false
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:     msg,
		StatusCode:  400,
		IsRetryable: &retryable,
	})
}

var fastRetry = retryConfig{
	maxRetries:     2,
	initialDelayMs: 1,
	backoffFactor:  1,
}

func TestRetryWithExponentialBackoff_SuccessOnFirstAttempt(t *testing.T) {
	calls := 0
	result, err := retryWithExponentialBackoff(context.Background(), fastRetry, func() (string, error) {
		calls++
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, 1, calls)
}

func TestRetryWithExponentialBackoff_SuccessAfterRetry(t *testing.T) {
	calls := 0
	result, err := retryWithExponentialBackoff(context.Background(), fastRetry, func() (string, error) {
		calls++
		if calls < 3 {
			return "", retryableAPIError("transient", nil)
		}
		return "recovered", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "recovered", result)
	assert.Equal(t, 3, calls)
}

func TestRetryWithExponentialBackoff_MaxRetriesExceeded(t *testing.T) {
	calls := 0
	_, err := retryWithExponentialBackoff(context.Background(), fastRetry, func() (string, error) {
		calls++
		return "", retryableAPIError(fmt.Sprintf("err-%d", calls), nil)
	})
	require.Error(t, err)
	assert.Equal(t, 3, calls)

	var retryErr *RetryError
	require.ErrorAs(t, err, &retryErr)
	assert.Equal(t, RetryMaxRetriesExceeded, retryErr.Reason)
	assert.Len(t, retryErr.Errors, 3)
}

func TestRetryWithExponentialBackoff_NonRetryableFirstAttempt(t *testing.T) {
	sentinel := errors.New("permanent")
	_, err := retryWithExponentialBackoff(context.Background(), fastRetry, func() (string, error) {
		return "", sentinel
	})
	require.ErrorIs(t, err, sentinel)

	var retryErr *RetryError
	assert.False(t, errors.As(err, &retryErr), "first-attempt non-retryable should not be wrapped")
}

func TestRetryWithExponentialBackoff_NonRetryableAfterRetryStarted(t *testing.T) {
	calls := 0
	cfg := retryConfig{maxRetries: 3, initialDelayMs: 1, backoffFactor: 1}
	_, err := retryWithExponentialBackoff(context.Background(), cfg, func() (string, error) {
		calls++
		if calls == 1 {
			return "", retryableAPIError("transient", nil)
		}
		return "", errors.New("permanent")
	})
	require.Error(t, err)
	assert.Equal(t, 2, calls)

	var retryErr *RetryError
	require.ErrorAs(t, err, &retryErr)
	assert.Equal(t, RetryErrorNotRetryable, retryErr.Reason)
	assert.Len(t, retryErr.Errors, 2)
}

func TestRetryWithExponentialBackoff_RetryDisabled(t *testing.T) {
	sentinel := retryableAPIError("retryable but disabled", nil)
	cfg := retryConfig{maxRetries: 0, initialDelayMs: 1, backoffFactor: 1}
	_, err := retryWithExponentialBackoff(context.Background(), cfg, func() (string, error) {
		return "", sentinel
	})
	require.ErrorAs(t, err, new(*provider.APICallError))

	var retryErr *RetryError
	assert.False(t, errors.As(err, &retryErr), "maxRetries=0 should not wrap")
}

func TestRetryWithExponentialBackoff_ContextCancelledDuringDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	cfg := retryConfig{maxRetries: 5, initialDelayMs: 5000, backoffFactor: 2}
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := retryWithExponentialBackoff(ctx, cfg, func() (string, error) {
		calls++
		return "", retryableAPIError("transient", nil)
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls, "should cancel during first backoff delay")
}

func TestRetryWithExponentialBackoff_NonRetryableAPIErrorStopsImmediately(t *testing.T) {
	calls := 0
	_, err := retryWithExponentialBackoff(context.Background(), fastRetry, func() (string, error) {
		calls++
		return "", nonRetryableAPIError("not retryable")
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetryError_Unwrap(t *testing.T) {
	sentinel := errors.New("inner")
	re := &RetryError{
		Reason: RetryMaxRetriesExceeded,
		Errors: []error{errors.New("first"), sentinel},
	}
	assert.ErrorIs(t, re, sentinel)
}

func TestGetRetryDelay_RetryAfterMs(t *testing.T) {
	apiErr := retryableAPIError("rate limited", map[string][]string{"Retry-After-Ms": {"500"}})
	d := getRetryDelay(apiErr, 2*time.Second)
	assert.Equal(t, 500*time.Millisecond, d)
}

func TestGetRetryDelay_RetryAfterSeconds(t *testing.T) {
	apiErr := retryableAPIError("rate limited", map[string][]string{"Retry-After": {"3"}})
	d := getRetryDelay(apiErr, 2*time.Second)
	assert.Equal(t, 3*time.Second, d)
}

func TestGetRetryDelay_UnreasonableValueIgnored(t *testing.T) {
	apiErr := retryableAPIError("rate limited", map[string][]string{"Retry-After-Ms": {"120000"}})
	exponential := 4 * time.Second
	d := getRetryDelay(apiErr, exponential)
	assert.Equal(t, exponential, d)
}

func TestGetRetryDelay_NoHeaders(t *testing.T) {
	apiErr := retryableAPIError("transient", nil)
	d := getRetryDelay(apiErr, 2*time.Second)
	assert.Equal(t, 2*time.Second, d)
}

func TestGetRetryDelay_RetryAfterMsTakesPriority(t *testing.T) {
	apiErr := retryableAPIError("rate limited", map[string][]string{
		"Retry-After-Ms": {"500"},
		"Retry-After":    {"10"},
	})
	d := getRetryDelay(apiErr, 2*time.Second)
	assert.Equal(t, 500*time.Millisecond, d)
}

func TestGetRetryDelay_UnreasonableMsDoesNotFallThroughToRetryAfter(t *testing.T) {
	apiErr := retryableAPIError("rate limited", map[string][]string{
		"Retry-After-Ms": {"120000"},
		"Retry-After":    {"3"},
	})
	exponential := 4 * time.Second
	d := getRetryDelay(apiErr, exponential)
	assert.Equal(t, exponential, d, "unreasonable retry-after-ms should use exponential delay, not fall through to retry-after")
}

func TestSleepWithContext_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := sleepWithContext(ctx, time.Hour)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSleepWithContext_ZeroDuration(t *testing.T) {
	err := sleepWithContext(context.Background(), 0)
	require.NoError(t, err)
}
