package aisdk

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

const (
	defaultMaxRetries  = 2
	maxReasonableDelay = 60 * time.Second
)

type retryConfig struct {
	maxRetries     int
	initialDelayMs float64
	backoffFactor  float64
}

var defaultRetryConfig = retryConfig{
	maxRetries:     defaultMaxRetries,
	initialDelayMs: 2000,
	backoffFactor:  2,
}

func retryWithExponentialBackoff[T any](
	ctx context.Context,
	cfg retryConfig,
	f func() (T, error),
) (T, error) {
	delayMs := cfg.initialDelayMs
	var errs []error

	for {
		result, err := f()
		if err == nil {
			return result, nil
		}

		if ctx.Err() != nil {
			var zero T
			return zero, ctx.Err()
		}

		if cfg.maxRetries == 0 {
			var zero T
			return zero, err
		}

		errs = append(errs, err)
		tryNumber := len(errs)

		if tryNumber > cfg.maxRetries {
			var zero T
			return zero, &RetryError{
				Reason: RetryMaxRetriesExceeded,
				Errors: errs,
			}
		}

		var apiErr *provider.APICallError
		if errors.As(err, &apiErr) && apiErr.IsRetryable && tryNumber <= cfg.maxRetries {
			delay := getRetryDelay(apiErr, time.Duration(delayMs*float64(time.Millisecond)))
			if err := sleepWithContext(ctx, delay); err != nil {
				var zero T
				return zero, err
			}
			delayMs *= cfg.backoffFactor
			continue
		}

		if tryNumber == 1 {
			var zero T
			return zero, err
		}

		var zero T
		return zero, &RetryError{
			Reason: RetryErrorNotRetryable,
			Errors: errs,
		}
	}
}

func getRetryDelay(apiErr *provider.APICallError, exponentialDelay time.Duration) time.Duration {
	headers := apiErr.ResponseHeaders
	if headers == nil {
		return exponentialDelay
	}

	h := http.Header(headers)

	var ms *time.Duration

	if vals := h.Values("retry-after-ms"); len(vals) > 0 {
		if parsed, parseErr := strconv.ParseFloat(vals[0], 64); parseErr == nil && !math.IsNaN(parsed) {
			d := time.Duration(parsed * float64(time.Millisecond))
			ms = &d
		}
	}

	if ms == nil {
		if vals := h.Values("retry-after"); len(vals) > 0 {
			if secs, parseErr := strconv.ParseFloat(vals[0], 64); parseErr == nil && !math.IsNaN(secs) {
				d := time.Duration(secs * float64(time.Second))
				ms = &d
			} else if t, parseErr := time.Parse(time.RFC1123, vals[0]); parseErr == nil {
				d := time.Until(t)
				ms = &d
			}
		}
	}

	if ms != nil && isReasonableDelay(*ms, exponentialDelay) {
		return *ms
	}

	return exponentialDelay
}

func isReasonableDelay(d, exponentialDelay time.Duration) bool {
	return d >= 0 && (d < maxReasonableDelay || d < exponentialDelay)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
