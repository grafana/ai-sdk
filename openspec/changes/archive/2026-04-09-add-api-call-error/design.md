## Context

The ai-sdk currently has no structured error type for API call failures. Errors from provider SDKs (e.g. `anthropic-sdk-go`) pass through as raw `error` values. Consumers like the fallback decider resort to fragile `strings.Contains` on `err.Error()` to classify errors.

The upstream TypeScript SDK defines `APICallError` in `@ai-sdk/provider` with fields for status code, retryability, response metadata, and provider-specific data. All upstream providers wrap their HTTP errors into this type, and orchestration code (`streamText`, fallback) consumes it for structured decisions.

The `anthropic-sdk-go` already provides rich error types (`anthropic.Error` with `StatusCode`, plus typed variants like `RateLimitError`, `OverloadedError`) but the Go ai-sdk never inspects them.

## Goals / Non-Goals

**Goals:**
- Define `APICallError` in `provider/` with full field parity to the upstream TypeScript `APICallError`.
- Wrap `anthropic-sdk-go` errors into `APICallError` in both generate and stream paths.
- Replace string-matching fallback decider with structured `IsRetryable` inspection.
- Maintain Go error chain semantics (`errors.As`, `errors.Is`, `Unwrap`).

**Non-Goals:**
- Adding error types beyond `APICallError` (e.g. `InvalidPromptError`, `TypeValidationError`) — those can follow later.
- Changing the `provider.LanguageModel` interface signature — errors are still returned as `error`.
- Adding retry logic to the provider layer — retryability is a classification signal, not behavior.

## Decisions

### 1. Type definition: struct with pointer receiver

```go
type APICallError struct {
    StatusCode      int
    URL             string
    RequestBodyValues any
    ResponseHeaders map[string][]string
    ResponseBody    string
    IsRetryable     bool
    Data            json.RawMessage
    message         string
    cause           error
}
```

**Rationale**: Full parity with upstream. `ResponseHeaders` uses Go's native `http.Header` shape (`map[string][]string`) instead of upstream's `Record<string, string>` — this preserves multi-value headers and avoids lossy conversion. `Data` is `json.RawMessage` for opaque provider-specific error payloads, matching the upstream `data?: unknown` pattern in a Go-idiomatic way. `message` and `cause` are unexported to match Go convention where `Error()` and `Unwrap()` are the public API.

**Alternative considered**: Minimal struct (just `StatusCode`, `IsRetryable`, `Message`, `Cause`). Rejected because adding fields later doesn't break callers but missing fields means consumers can't access response metadata that upstream exposes.

### 2. Constructor function with default retryability

```go
func NewAPICallError(opts APICallErrorOptions) *APICallError
```

With an options struct:

```go
type APICallErrorOptions struct {
    Message         string
    URL             string
    RequestBodyValues any
    StatusCode      int
    ResponseHeaders map[string][]string
    ResponseBody    string
    IsRetryable     *bool  // nil → auto-compute from status code
    Data            json.RawMessage
    Cause           error
}
```

Default `IsRetryable` when nil: `true` for 408, 409, 429, >= 500. This matches the upstream constructor defaults exactly.

**Rationale**: Using `*bool` for `IsRetryable` distinguishes "caller didn't specify" (auto-compute) from "caller explicitly set false". A plain `bool` would make it impossible to auto-compute since the zero value is a valid explicit choice.

**Alternative considered**: Functional options (`WithStatusCode(n)`, `WithRetryable(b)`). Rejected as heavier than needed for a simple error constructor; the upstream uses a plain options object.

### 3. Error interface implementation

```go
func (e *APICallError) Error() string  // "aisdk: API call error (status 429): rate limit exceeded"
func (e *APICallError) Unwrap() error  // returns e.cause for errors.As/errors.Is chain
```

**Rationale**: Standard Go error wrapping. `Error()` includes status code and message for logging ergonomics. The cause is accessible via `Unwrap()` so consumers can `errors.As` into the original SDK error (`anthropic.Error`, etc.) when they need provider-specific details.

### 4. Anthropic error wrapping

In `anthropic/model.go`, create a helper:

```go
func wrapAPIError(err error, url string, body any) error
```

This inspects `err` with `errors.As(err, &apierr)` where `apierr` is `*anthropic.Error`, extracts `StatusCode`, raw JSON body, and response headers, then wraps into `*provider.APICallError`.

Applied in two places:
- `DoGenerate`: wrap the error from `m.client.Beta.Messages.New()`
- `consumeStream`: wrap `stream.Err()` before emitting as `PartError`

For stream-level errors from SSE events (e.g. `overloaded_error`), the upstream maps these to synthetic status codes (529 for overloaded, 500 otherwise). The Go port will follow the same mapping when the `anthropic-sdk-go` stream error contains typed error information.

**Rationale**: Centralizing wrapping in a helper avoids duplication between generate and stream paths. The upstream anthropic provider does the same wrapping via `failedResponseHandler`.

### 5. Fallback decider replacement

Replace:
```go
func defaultDecider(err error) bool {
    msg := strings.ToLower(err.Error())
    for _, pattern := range contextLengthPatterns {
        if strings.Contains(msg, pattern) { return false }
    }
    return true
}
```

With:
```go
func defaultDecider(err error) bool {
    var apiErr *provider.APICallError
    if errors.As(err, &apiErr) {
        return apiErr.IsRetryable
    }
    return true
}
```

**Rationale**: Matches upstream behavior. Status-code-driven classification is more reliable than substring matching. Context-length errors (400) are automatically non-retryable. Unknown errors (non-`APICallError`) default to `true` (try next candidate), same as current behavior.

### 6. Package placement

`APICallError` lives in `provider/` (the leaf package with zero deps on root). This matches upstream where `APICallError` lives in `@ai-sdk/provider`. The `anthropic/` module already depends on `provider/`, so no new dependency edges are introduced.

## Risks / Trade-offs

- **[Behavioral change in fallback]** The decider now uses `IsRetryable` instead of context-length patterns. A 400 error that isn't context-length-related but was previously retried will now *not* trigger fallback. → This matches upstream semantics and is the desired behavior; non-retryable errors should not trigger fallback.
- **[Breaking for error type assertion]** Consumers doing `err.(*anthropic.Error)` directly on provider errors will break because the outer type is now `*provider.APICallError`. → Mitigated by `Unwrap()` — consumers switch to `errors.As(err, &anthropicErr)` which traverses the chain. This is the standard Go pattern and should be documented in the change notes.
- **[Stream error fidelity]** `anthropic-sdk-go` stream errors may not always be typed `anthropic.Error` (e.g. network errors, JSON parse failures). → Non-`anthropic.Error` errors are passed through without wrapping, matching current behavior. Only API errors with status codes get wrapped.
