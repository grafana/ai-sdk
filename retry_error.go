package aisdk

import "fmt"

// RetryErrorReason describes why retry stopped.
type RetryErrorReason string

const (
	RetryMaxRetriesExceeded RetryErrorReason = "maxRetriesExceeded"
	RetryErrorNotRetryable  RetryErrorReason = "errorNotRetryable"
)

// RetryError is returned when all retry attempts have been exhausted or
// a non-retryable error is encountered after retries have already started.
// It carries every error from each attempt and the reason retries stopped.
type RetryError struct {
	Reason RetryErrorReason
	Errors []error
}

func (e *RetryError) LastError() error {
	if len(e.Errors) == 0 {
		return nil
	}
	return e.Errors[len(e.Errors)-1]
}

func (e *RetryError) Error() string {
	n := len(e.Errors)
	last := e.LastError()
	switch e.Reason {
	case RetryMaxRetriesExceeded:
		return fmt.Sprintf("aisdk: failed after %d attempts. Last error: %v", n, last)
	case RetryErrorNotRetryable:
		return fmt.Sprintf("aisdk: failed after %d attempts with non-retryable error: %v", n, last)
	default:
		return fmt.Sprintf("aisdk: retry failed after %d attempts: %v", n, last)
	}
}

func (e *RetryError) Unwrap() error { return e.LastError() }
