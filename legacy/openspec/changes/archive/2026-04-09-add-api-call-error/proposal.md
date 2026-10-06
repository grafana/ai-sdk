## Why

The ai-sdk currently passes errors through as raw `error` values with no structure, making it impossible for consumers (fallback decider, retry logic, error reporting) to programmatically inspect HTTP status codes or retryability. The upstream TypeScript SDK defines `APICallError` in `@ai-sdk/provider` as the standard provider-agnostic error type — the Go port needs parity.

## What Changes

- Add `APICallError` struct in `provider/` mirroring the upstream `APICallError` class with full field parity: `StatusCode`, `IsRetryable`, `URL`, `RequestBodyValues`, `ResponseHeaders`, `ResponseBody`, `Data`, `Message`, and wrapped `Cause`.
- Default `IsRetryable` logic: `true` for status codes 408, 409, 429, and >= 500.
- Wrap errors from `anthropic-sdk-go` into `APICallError` in both `DoGenerate` and `consumeStream` paths, extracting `StatusCode` and response body from `anthropic.Error`.
- Replace the string-matching `defaultDecider` in `fallback/` with structured `APICallError` inspection using `IsRetryable`.

## Capabilities

### New Capabilities
- `api-call-error`: Structured error type for provider API call failures, with status code, retryability classification, response metadata, and Go error wrapping.

### Modified Capabilities

## Impact

- `provider/`: New `APICallError` type and constructor. No breaking changes — this is additive.
- `anthropic/model.go`: Error wrapping in `DoGenerate` return path and `consumeStream` goroutine. Errors returned/emitted change from raw `anthropic-sdk-go` errors to `*provider.APICallError` wrapping the original.
- `fallback/fallback.go`: `defaultDecider` implementation changes from string matching to `errors.As` with `APICallError`. Behavioral change: decisions are now based on `IsRetryable` (status-code-driven) instead of context-length pattern matching. The set of "don't fallback" errors shifts from substring heuristics to HTTP 4xx (non-retryable) semantics.
- Consumers using `errors.As` or `errors.Is` on provider errors will see `*provider.APICallError` as the outer type with the original SDK error accessible via `Unwrap()`.
