## Context

Issue #419 lists the stream worker goroutines on the Gateway path. ProviderWire already recovers its own selection, invocation and stream-setup workers, and `fallback` recovers its stream-setup worker with `errStreamSetupPanic`.

## Decisions

- **Shared shape.** A root `internal/` package, because the providers and middleware already import root internal packages (`internal/streamusage`) and #419 rules out new public API without review. `Recover` must be deferred directly, since `recover` stops a panic only when the deferred function calls it.
- **Error shape.** Stream error parts must carry an `*APICallError`. The recovered part uses status 0 and `IsRetryable: false`, because a worker that panicked mid-stream may already have produced output or effects. Its cause is `ErrPanic`, so consumers detect it with `errors.Is`. In anthropic and openai, which read the first event before returning, a panic before it makes `DoStream` return the plain sentinel, which the Gateway already classifies as internal. openai-compatible returns its stream first, so the same panic arrives as the terminal error part.
- **Gateway.** Both Gateway classifiers map a status-0 `APICallError` to an upstream failure. The streaming runtime checks for `ErrPanic` first and treats the part as an adapter failure: one internal error frame, then cancellation. Unary already recovers `DoGenerate`.
- **OpenAI retryability.** The openai transport wrapper marks unknown errors retryable. For the sentinel it returns the non-retryable recovered error instead, and its preflight passes the sentinel through like context errors.
- **Middleware.** Observation is contained where user callbacks or exporters can panic: the logger's clock and sinks, Prometheus finalization (normalizers and label values) and Agent Observability finalization (agento11y and its tracer). After an observation panic the stream keeps forwarding and the remaining observation for that stream is skipped. agento11y labels any call error `provider_call_error` with category `sdk_error` for status 0, wrapped or not, so the recording middleware does not change it.
- **Not guarded.** The `fallback` forwarders and the providers' outer forwarding loops only move parts between channels and call no provider or user code, so no test can make them panic. Bedrock, Azure and the Grafana client are not on the Gateway path today.

## Upstream

At the registered baseline (`eb77f09`), `@ai-sdk/openai-compatible` builds its stream with `response.pipeThrough(new TransformStream({ transform(chunk, controller) { ... } }))` (`openai-compatible-chat-language-model.ts`). An exception thrown in `transform` errors that stream's readable side only. Ending the stream with a terminal error part is the parity-preserving Go adaptation.
