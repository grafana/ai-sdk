## Context

`anthropic-sdk-go` v1.78.0, pinned on `main`, has the same guard as v1.75.0: `BetaMessageService.New` calls `CalculateNonStreamingTimeout(maxTokens, model, opts)`, which returns a configured `RequestTimeout` as is, refuses when `time.Hour * maxTokens / 128000` exceeds ten minutes or the model's non-streaming token limit is exceeded, and otherwise returns a ten-minute default. Upstream `@ai-sdk/anthropic` uses `postJsonToApi` with the caller's abort signal and has no such check.

## Decisions

### 1. Ask the SDK instead of copying its rule

Calling the exported `CalculateNonStreamingTimeout` with the same options the request uses means the provider intervenes exactly when the SDK would refuse, and tracks any change to the SDK's thresholds. User options from `WithRequestOptions` are per-request options, so a caller's timeout is visible to the check and wins.

### 2. Which timeout

The SDK needs some timeout to proceed. The caller's context deadline is the most honest value when present, since the request cannot outlive it anyway. It is rounded up to one second because the SDK sends the timeout in whole seconds and a sub-second remainder would go out as 0. Without one, the SDK's own estimate for the request, at least ten minutes, keeps the timeout proportional to `max_tokens` instead of inventing a constant. A model whose non-streaming token limit is exceeded now reaches the API, which answers for itself, as it does for upstream.
