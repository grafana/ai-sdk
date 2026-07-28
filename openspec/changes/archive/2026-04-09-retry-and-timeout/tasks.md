## 1. Retryable Error Interface

- [x] 1.1 Add `RetryableError` interface to `provider` package with `IsRetryable() bool` method
- [x] 1.2 Add optional `ResponseHeaders() map[string][]string` method for rate-limit header access (separate interface, checked via type assertion)
- [x] 1.3 Update Anthropic provider to wrap API errors with `RetryableError` implementation (retryable for 408, 409, 429, >=500)

## 2. RetryError Type

- [x] 2.1 Add `RetryError` type in `aisdk` package with `Reason` (enum: `MaxRetriesExceeded`, `ErrorNotRetryable`), `Errors []error`, and `LastError()` accessor
- [x] 2.2 Implement `Error() string`, `Unwrap() error` (returns last error) on `RetryError`

## 3. Retry Logic

- [x] 3.1 Implement `retryWithExponentialBackoff` function: accepts a callable, maxRetries, and context; returns result or error
- [x] 3.2 Implement exponential backoff delay calculation (initial 2s, factor 2x)
- [x] 3.3 Implement cancellable delay using `time.NewTimer` + `select` on `ctx.Done()`
- [x] 3.4 Implement rate-limit header parsing (`retry-after-ms` then `retry-after`) with reasonableness check (0 <= value AND (value < 60s OR value < exponential delay))
- [x] 3.5 Implement error wrapping rules: unwrapped for maxRetries=0, unwrapped for first-attempt non-retryable, `RetryError` for exhausted/non-retryable-after-started
- [x] 3.6 Add tests for retry logic: successful retry, exhausted retries, non-retryable errors, context cancellation during delay, rate-limit header parsing

## 4. Timeout Configuration

- [x] 4.1 Add `TimeoutConfig` struct with `Total`, `Step`, `Chunk` as `time.Duration` fields
- [x] 4.2 Add `Timeout` functional option to `StreamOption`/`GenerateOption`
- [x] 4.3 Add `MaxRetries` functional option to `StreamOption`/`GenerateOption` (default 2)

## 5. Timeout Integration in StreamText

- [x] 5.1 Apply total timeout: wrap caller context with `context.WithTimeout` at the top of `StreamText` when `Total` is set
- [x] 5.2 Apply step timeout: create per-step `time.AfterFunc` that aborts the operation, stopped at end of step
- [x] 5.3 Apply chunk timeout: implement `time.AfterFunc` timer in `processStep` that resets on each stream part and aborts operation on expiry
- [x] 5.4 Ensure chunk timeout is stopped/cleaned up when step finishes

## 6. Retry Integration in StreamText

- [x] 6.1 Wrap the `DoStream` call in `run()` with `retryWithExponentialBackoff` using the step context
- [x] 6.2 Ensure retry is re-created per step with the correct step context

## 7. GenerateText Integration

- [x] 7.1 Apply total and step timeouts in `GenerateText` (no chunk timeout)
- [x] 7.2 Wrap `DoGenerate` with retry logic (via `StreamText` since `GenerateText` delegates to it, or directly if needed)
- [x] 7.3 Verify that `MaxRetries` and `Timeout` options propagate through `GenerateText`'s config

## 8. Testing

- [x] 8.1 Add unit tests for timeout behavior: total timeout across steps, step timeout per invocation, chunk timeout stall detection
- [x] 8.2 Add unit tests for retry + timeout interaction: timeout cancels retry delay, timeout prevents further retries
- [x] 8.3 Add unit tests for per-step retry independence in multi-step tool loops
- [x] 8.4 Add integration test with mock model verifying end-to-end retry and timeout behavior
