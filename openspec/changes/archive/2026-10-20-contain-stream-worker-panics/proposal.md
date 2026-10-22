## Why

Go recovers a panic only on the goroutine that raised it. The worker goroutines that produce, forward and observe provider stream parts on the Gateway path do not recover, so one unexpected payload or bug in any of them terminates the process and every concurrent stream (#419). In the registered upstream baseline, an exception in a provider stream's transform errors only that stream.

## What Changes

- Add a root `internal/streamrecover` package: an `ErrPanic` sentinel, a terminal error part carrying a non-retryable status-0 `APICallError` that wraps it, and a `Recover` helper that discards the recovered value.
- The anthropic and openai transport readers and stream adapters, and the openai-compatible stream worker, end their stream with that part when they panic, then release upstream resources through their existing ownership paths. In anthropic and openai, a panic before `DoStream` returns fails it with the plain sentinel; openai-compatible, which returns its stream first, ends that stream with the terminal part.
- The Gateway ends a stream with one internal error frame when it receives that part.
- The logger, Prometheus and Agent Observability stream tees contain panics in their observation code and keep forwarding the stream. The logger reports the sentinel as `internal_error` and Prometheus as `other`.

## Capabilities

### New Capabilities
- `stream-worker-panic-containment`: Panic containment for stream workers on the Gateway path.

## Impact

`internal/streamrecover` (new), `providers/anthropic`, `providers/openai`, `providers/openai-compatible`, `middleware/logger`, `middleware/prometheus`, `middleware/agentobservability` and `ai-gateway/providerwire/v4`. No public API changes. The Gateway imports a root `internal/` package for the first time, as the providers and middleware already do. Normal streams, cancellation, drain limits and fallback behavior are unchanged.
