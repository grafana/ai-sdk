## Context

The Go port currently has no retry or timeout logic at the orchestration level. When `DoStream`/`DoGenerate` returns an error, `StreamText` emits it and stops immediately. The only resilience mechanism is `fallback.Model`, which tries alternative models but does not retry a single model on transient errors.

Upstream Vercel AI SDK wraps every `doStream`/`doGenerate` call with `retryWithExponentialBackoffRespectingRetryHeaders()` and layers `AbortSignal.timeout()` for total, step, and chunk timeouts. These are orthogonal: timeout fires abort, abort kills retry immediately.

Key upstream files:
- `packages/ai/src/util/retry-with-exponential-backoff.ts` -- retry algorithm
- `packages/ai/src/util/retry-error.ts` -- structured error type
- `packages/ai/src/core/generate-text/stream-text.ts` -- timeout + retry integration
- `packages/provider/src/errors/api-call-error.ts` -- retryable classification

## Goals / Non-Goals

**Goals:**
- Wire-compatible retry semantics with upstream (default maxRetries=2, exponential backoff, rate-limit header respect)
- Timeout configuration matching upstream's three levels (total, step, chunk) using Go-idiomatic `context.Context` + `time.Duration`
- Retryable error interface in the provider package for classification
- Structured `RetryError` for exhausted-retry reporting
- Works with both `StreamText` and `GenerateText`
- Retry wraps individual model invocations (per-step), not the entire multi-step loop

**Non-Goals:**
- Tool execution timeouts (`toolMs` / per-tool overrides from upstream) -- separate concern, can be added later
- HTTP-level retry inside providers -- that's the provider's responsibility (e.g., anthropic-sdk-go has its own)
- Custom retry strategies or pluggable backoff functions
- Jitter on backoff delays (upstream doesn't use jitter)

## Decisions

### 1. Retryable error detection via `provider.APICallError`

**Decision**: Use the concrete `*provider.APICallError` type for retryable error detection. The retry logic checks `errors.As(err, &apiErr)` and reads `apiErr.IsRetryable` as a struct field. This aligns with the upstream `APICallError` pattern and reuses the type introduced in PR #134.

**Alternatives considered**:
- Interface-based detection (`RetryableError` with `IsRetryable() bool`): was the original approach, but `APICallError` already provides both retryability and response headers as struct fields, making the interface layer redundant
- String matching on error messages: fragile, not extensible
- Always-retry on any error: dangerous, could retry non-idempotent failures

**Rationale**: `APICallError` is the canonical error type for provider API failures, matching upstream's `@ai-sdk/provider` `APICallError`. Using `errors.As` with the concrete type is idiomatic Go and avoids an unnecessary interface layer. The fallback package also uses `APICallError` for its decider, keeping the error classification pattern consistent across the SDK.

### 2. Timeout via layered context cancellation

**Decision**: Layer context cancellation with timers:
- Total timeout: `context.WithTimeout` set once at the top of `StreamText`/`GenerateText`, wraps the caller's context
- Operation cancel: a single `context.WithCancel` derived from the total context, providing an `opCancel` function that step and chunk timers both fire
- Step timeout: per-step `time.AfterFunc` that calls `opCancel` on expiry, stopped at end of each step
- Chunk timeout: rolling `time.AfterFunc` timer reset on each stream part, calls `opCancel` on expiry

**Alternatives considered**:
- Single merged context like upstream's `mergeAbortSignals`: Go contexts already compose via parent-child, no need for a merge utility
- Per-step `context.WithTimeout`: creates unnecessary context layers; `time.AfterFunc` + shared `opCancel` is simpler and matches upstream's "once aborted, permanently aborted" behavior

**Rationale**: Go's context tree naturally handles the "any timeout kills everything below" behavior. Total context is parent of the operation context, so total expiry cancels everything. Step and chunk timers share `opCancel`, matching upstream where `stepAbortController` and `chunkAbortController` are created once and permanently abort the merged signal.

### 3. Retry config built once, invoked per-step

**Decision**: The retry config is built once before the step loop (since it's derived from immutable `baseConfig`). The retry function is *invoked* fresh for each step with the current step's context. This matches upstream where `prepareRetries` is called inside `streamStep`.

**Rationale**: Each step shares the same retry parameters but has a different context (step timeout resets per step). The retry's delay must respect the current step's cancellation, not a stale context from a previous step.

### 4. Rate-limit header access via APICallError.ResponseHeaders

**Decision**: The retry logic reads `retry-after-ms` and `retry-after` directly from the `APICallError.ResponseHeaders` struct field. Since `getRetryDelay` receives the `*provider.APICallError` directly (after the `errors.As` check), no interface or type assertion is needed.

**Alternatives considered**:
- Separate `RetryableErrorWithHeaders` interface: was the original approach, but redundant once `APICallError` provides `ResponseHeaders` as a field
- Parse headers from a raw `http.Response`: too coupled to HTTP transport

**Rationale**: `APICallError.ResponseHeaders` is populated by providers when wrapping SDK errors (e.g., from `anthropic/wrap_api_error.go`). The retry logic reads the field directly, falling back to pure exponential backoff when headers are nil.

### 5. RetryError as a concrete type in aisdk package

**Decision**: `RetryError` lives in the root `aisdk` package (not provider), since it's an orchestration concern. It carries a reason enum (`MaxRetriesExceeded` / `ErrorNotRetryable`), all attempt errors, and exposes `LastError()` and `Unwrap()`.

**Rationale**: Retry is orchestration logic, not provider logic. The error type follows Go conventions: `Error()` returns a message, `Unwrap()` returns the last error for `errors.Is`/`errors.As` chaining.

### 6. Error wrapping rules match upstream exactly

**Decision**: Follow upstream's nuanced wrapping behavior:
- `maxRetries=0`: errors pass through unwrapped
- First attempt + non-retryable: error passes through unwrapped
- Retries exhausted: `RetryError{Reason: MaxRetriesExceeded}`
- Non-retryable after retries started: `RetryError{Reason: ErrorNotRetryable}`
- Context cancelled during retry: return context error immediately, never wrap

**Rationale**: Wire compatibility. Consumers may rely on error type for different handling.

### 7. Chunk timeout uses time.AfterFunc, not a goroutine

**Decision**: Use `time.AfterFunc` for chunk stall detection, reset via `timer.Reset()` on each stream part. On expiry, call `cancel()` on the step context.

**Alternatives considered**:
- Separate goroutine with a `time.Timer` and channel `select`: more code, same semantics
- Wrapping the stream channel with a timeout-aware reader: invasive to stream consumption

**Rationale**: `time.AfterFunc` is the most minimal approach -- no extra goroutine management, just reset on each chunk.

## Risks / Trade-offs

- **[Risk] Retry masking real errors** -- A misconfigured retryable interface could cause retries on non-idempotent operations. Mitigation: only retry if the error explicitly opts in via `IsRetryable() bool`, and document that providers must be careful about what they mark retryable.

- **[Risk] Chunk timeout false positives** -- Large model responses with slow token generation could trigger chunk timeout. Mitigation: chunk timeout is optional and off by default. Users must explicitly configure it for their use case.

- **[Risk] Anthropic SDK's own retry** -- The `anthropic-sdk-go` library has built-in retry. Having retry at both layers could cause multiplicative attempts (e.g., 3 * 3 = 9 total calls). Mitigation: document this interaction, consider disabling the SDK-level retry when orchestration retry is enabled, or set provider maxRetries=0.

- **[Trade-off] No jitter** -- Upstream doesn't use jitter on backoff. This means concurrent clients hitting the same rate limit will retry in lockstep. Acceptable for now to match upstream; can add jitter later if needed.

- **[Trade-off] Step/chunk timeout aborts entire operation** -- Matching upstream behavior where the abort controllers are created once and shared across steps. A step timeout doesn't just skip that step, it kills the whole operation. This is intentional for predictable timeout guarantees.
