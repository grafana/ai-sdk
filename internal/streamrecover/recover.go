// Package streamrecover contains panics in stream worker goroutines, so a
// failure while producing, forwarding or observing one stream ends only that
// stream. The recovered value is always discarded: it may hold request data
// or credentials.
package streamrecover

import (
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

// ErrPanic marks a stream that ended because one of its workers panicked.
var ErrPanic = errors.New("aisdk: stream worker panicked")

// Error returns a new non-retryable APICallError, without a status code,
// whose cause is ErrPanic.
func Error() *provider.APICallError {
	retryable := false
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:     "internal stream failure",
		IsRetryable: &retryable,
		Cause:       ErrPanic,
	})
}

// Part returns a terminal error part carrying Error.
func Part() provider.StreamPart {
	return provider.StreamPart{Type: provider.PartError, APICallError: Error()}
}

// Recover calls onPanic when the goroutine is panicking. It must be deferred
// directly, as in defer streamrecover.Recover(f); recover only stops a panic
// when called by the deferred function itself.
func Recover(onPanic func()) {
	if recover() != nil {
		onPanic()
	}
}
