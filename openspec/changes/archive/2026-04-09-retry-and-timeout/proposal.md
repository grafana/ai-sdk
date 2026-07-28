## Why

The upstream Vercel AI SDK retries failed provider calls with exponential backoff and supports granular timeout configuration. Our Go port has neither at the orchestration level -- every consumer must build their own resilience layer. The `fallback.Model` provides model-level failover but not retry-on-transient-error for a single model.

## What Changes

- Add **retry with exponential backoff** wrapping each `DoStream`/`DoGenerate` call:
  - `MaxRetries` option (default 2, so up to 3 total attempts; 0 disables)
  - Exponential backoff: initial delay 2s, factor 2x
  - Only retry errors that implement a retryable interface (transient API errors, rate limits)
  - Respect `retry-after-ms` and `retry-after` response headers when reasonable
  - Return structured `RetryError` when retries are exhausted, carrying all attempt errors
- Add **timeout configuration** with three granularity levels:
  - `TotalTimeout`: aborts entire operation (all steps) if exceeded
  - `StepTimeout`: aborts if a single model invocation exceeds this duration
  - `ChunkTimeout`: aborts if no new stream chunk arrives within duration (stall detection)
- Add **retryable error interface** to the provider package so providers can signal which errors are transient
- Retry wraps each individual model invocation, so in multi-step tool loops each step retries independently
- Timeouts use `context.Context` cancellation, which naturally cancels in-progress retry delays

## Capabilities

### New Capabilities

- `retry`: Exponential backoff retry logic for model invocations with rate-limit header respect and structured error reporting
- `timeout`: Multi-level timeout configuration (total, per-step, per-chunk) using context cancellation

### Modified Capabilities

(none)

## Impact

- **aisdk package**: `StreamText` and `GenerateText` gain retry wrapping and timeout context management in the orchestration loop
- **provider package**: New `RetryableError` interface for providers to classify transient errors; new `ResponseHeaders` usage for retry-after headers
- **anthropic module**: May need to surface retryable status from the Anthropic SDK's error types
- **Options surface**: New `MaxRetries` and `Timeout` functional options added to `StreamOption`/`GenerateOption`
- **New error types**: `RetryError` in aisdk package for exhausted-retry reporting
