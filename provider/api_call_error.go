package provider

import (
	"encoding/json"
	"fmt"
)

// APICallError represents a failed API call to a provider. It captures HTTP
// status code, retryability, response metadata, and wraps the original cause
// for in-process error chain traversal (errors.As / errors.Is).
//
// All fields except cause are exported with JSON tags so the error round-trips
// through encoding/json. The unexported cause is preserved for in-process
// Unwrap but is not serialized; reconstructed error attribution should use
// Message, StatusCode, and ResponseBody instead.
//
// This mirrors the upstream TypeScript APICallError from @ai-sdk/provider.
type APICallError struct {
	Message           string              `json:"message"`
	StatusCode        int                 `json:"statusCode"`
	URL               string              `json:"url,omitempty"`
	RequestBodyValues json.RawMessage     `json:"requestBodyValues,omitempty"`
	ResponseHeaders   map[string][]string `json:"responseHeaders,omitempty"`
	ResponseBody      string              `json:"responseBody,omitempty"`
	// IsRetryable reports whether the provider considers the failure eligible
	// for a fresh call. It does not make replaying an established stream safe;
	// callers must separately ensure no output or effects escaped the attempt.
	IsRetryable bool            `json:"isRetryable"`
	Data        json.RawMessage `json:"data,omitempty"`

	// cause is the in-process underlying error. It is not serialized to JSON
	// because Go errors are not generally serializable. Reconstructed errors
	// from JSON have a nil cause.
	cause error
}

var _ error = (*APICallError)(nil)

func (e *APICallError) Error() string {
	return fmt.Sprintf("aisdk: API call error (status %d): %s", e.StatusCode, e.Message)
}

func (e *APICallError) Unwrap() error {
	return e.cause
}

// APICallErrorOptions configures how an APICallError is created. IsRetryable
// is a *bool so that nil means "auto-compute from StatusCode".
type APICallErrorOptions struct {
	Message           string
	URL               string
	RequestBodyValues json.RawMessage
	StatusCode        int
	ResponseHeaders   map[string][]string
	ResponseBody      string
	IsRetryable       *bool
	Data              json.RawMessage
	Cause             error
}

// NewAPICallError creates an APICallError. When opts.IsRetryable is nil,
// retryability is inferred from the status code: 408, 409, 429, and >= 500
// are retryable; everything else is not.
func NewAPICallError(opts APICallErrorOptions) *APICallError {
	retryable := false
	if opts.IsRetryable != nil {
		retryable = *opts.IsRetryable
	} else {
		retryable = opts.StatusCode == 408 ||
			opts.StatusCode == 409 ||
			opts.StatusCode == 429 ||
			opts.StatusCode >= 500
	}

	return &APICallError{
		Message:           opts.Message,
		StatusCode:        opts.StatusCode,
		URL:               opts.URL,
		RequestBodyValues: opts.RequestBodyValues,
		ResponseHeaders:   opts.ResponseHeaders,
		ResponseBody:      opts.ResponseBody,
		IsRetryable:       retryable,
		Data:              opts.Data,
		cause:             opts.Cause,
	}
}
