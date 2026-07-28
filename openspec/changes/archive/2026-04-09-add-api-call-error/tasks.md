## 1. APICallError type in provider/

- [x] 1.1 Define `APICallError` struct in `provider/api_call_error.go` with all fields: `StatusCode`, `URL`, `RequestBodyValues`, `ResponseHeaders`, `ResponseBody`, `IsRetryable`, `Data`, unexported `message` and `cause`
- [x] 1.2 Implement `Error() string` method (format: `"aisdk: API call error (status %d): %s"`)
- [x] 1.3 Implement `Unwrap() error` method returning the cause
- [x] 1.4 Define `APICallErrorOptions` struct with `IsRetryable *bool` for optional override
- [x] 1.5 Implement `NewAPICallError(opts APICallErrorOptions) *APICallError` constructor with default retryability logic (408, 409, 429, >=500 → retryable)
- [x] 1.6 Add compile-time interface check: `var _ error = (*APICallError)(nil)`

## 2. APICallError unit tests

- [x] 2.1 Add `provider/api_call_error_test.go` with table-driven tests for default retryability (408, 409, 429, 500, 503 → true; 400, 401, 403 → false)
- [x] 2.2 Test explicit `IsRetryable` override (true overrides non-retryable status, false overrides retryable status)
- [x] 2.3 Test `Error()` output format contains status code and message
- [x] 2.4 Test `Unwrap()` returns cause and `errors.Is` / `errors.As` chain works

## 3. Anthropic provider error wrapping

- [x] 3.1 Add `wrapAPIError(err error, url string, body any) error` helper in `anthropic/` that inspects `anthropic.Error` via `errors.As`, extracts `StatusCode`, response body, and response headers, and wraps into `*provider.APICallError`
- [x] 3.2 Wrap error in `DoGenerate` after `m.client.Beta.Messages.New()` call
- [x] 3.3 Wrap `stream.Err()` in `consumeStream` before emitting `PartError`
- [x] 3.4 Ensure non-`anthropic.Error` errors (network, DNS) pass through unwrapped

## 4. Anthropic error wrapping tests

- [x] 4.1 Test `wrapAPIError` with a mock `anthropic.Error` (429) produces `APICallError` with correct `StatusCode` and `IsRetryable`
- [x] 4.2 Test `wrapAPIError` with a mock `anthropic.Error` (400) produces non-retryable `APICallError`
- [x] 4.3 Test `wrapAPIError` with a non-API error returns the error unwrapped
- [x] 4.4 Test that `Unwrap` on the wrapped error returns the original `anthropic.Error`

## 5. Fallback decider update

- [x] 5.1 Replace `defaultDecider` in `fallback/fallback.go` with `errors.As`-based `APICallError` inspection using `IsRetryable`
- [x] 5.2 Remove `contextLengthPatterns` variable and `strings` import (if no longer needed)

## 6. Fallback decider tests

- [x] 6.1 Test decider returns `true` for `APICallError` with `IsRetryable: true`
- [x] 6.2 Test decider returns `false` for `APICallError` with `IsRetryable: false`
- [x] 6.3 Test decider returns `true` for non-`APICallError` errors (unknown error fallback)
- [x] 6.4 Test decider returns `false` for `APICallError` with `StatusCode: 400` (context-length class)
