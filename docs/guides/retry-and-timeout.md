# Retry and timeout

Model calls depend on remote services and should always have bounded latency.
Use retries for brief transient failures and timeouts to stop work that no
longer serves the caller.

## Configure retries

The orchestration layer defaults to two retries, for at most three attempts.
Only `provider.APICallError` values marked retryable are retried. Provider
responses such as `Retry-After` are honored when reasonable.

```go
result := aisdk.StreamText(ctx, model,
	aisdk.WithMaxRetries(2),
	// other options
)
```

Set `WithMaxRetries(0)` when another layer owns retry policy or when duplicate
requests are unacceptable. Retrying a model call can increase latency and cost;
it does not guarantee the same output.

## Bound the operation

```go
result := aisdk.StreamText(ctx, model,
	aisdk.WithTimeout(aisdk.TimeoutConfig{
		Total:      45 * time.Second,
		Step:       20 * time.Second,
		FirstChunk: 15 * time.Second,
		Chunk:      10 * time.Second,
	}),
)
```

- `Total` limits the complete operation, including tool steps and retry delays.
- `Step` limits one model call.
- `FirstChunk` limits how long each streaming step can wait for its first
  non-empty text, reasoning, tool-input delta, file, reasoning file, or tool
  call.
- `Chunk` limits the gap between those semantic output parts after output
  begins.

`FirstChunk` and `Chunk` apply only to `StreamText`. Non-content events such as
metadata, starts, ends, raw events, and empty deltas do not reset them. When the
effective tool choice is `none`, the SDK withholds tool input and tool calls
before checking these timeouts. Withheld events neither satisfy `FirstChunk`
nor reset `Chunk`.

All four timeouts are disabled unless configured. Choose values from the
calling endpoint's latency budget; the example values are illustrative.

Always pass the caller's context. Its cancellation or deadline composes with
SDK timeouts and interrupts retry waits.

When `StreamText` handles cancellation during provider startup or streaming, or
when `PrepareStep` returns an error after the operation context is canceled, it
calls `OnAbort` and emits `StreamAbort` (UI `abort`). The abort does not call
`OnError` or `OnFinish`. `Err()` reports the first recorded stream error, if any;
it can be nil after an abort. Watch for `OnAbort` or the abort event rather than
relying on `Err()` alone.

`StreamAbort.Reason` describes the context cause, not which timer fired. In
particular, `Step`, `FirstChunk`, and `Chunk` can all produce `context canceled`.
`GenerateText` and `Agent.Generate` return an error instead: the first recorded
stream error takes precedence, otherwise they return the error from the canceled
startup or the context cause if cancellation occurred while streaming.

## Avoid stacked retries

Provider clients may retry independently. The Anthropic Go client retries by
default, so enabling both layers can multiply attempts. Choose one owner:

- keep SDK retries when you want common behavior across providers; or
- disable SDK retries when provider-specific policy is intentional.

Also account for `fallback`: each candidate can have its own retry attempts.
A five-step agent with retries and fallback can make many more provider calls
than one user request suggests.

## Retry only safe work

The SDK retries provider calls, not completed tool side effects. Tool execution
must still be idempotent or protected with operation IDs when a surrounding
application may repeat the overall request.

Observe attempt counts, final error categories, latency, and usage so retry
policy can be tuned from real traffic.

## Reference

- [`WithMaxRetries`](https://pkg.go.dev/github.com/grafana/ai-sdk#WithMaxRetries)
- [`TimeoutConfig`](https://pkg.go.dev/github.com/grafana/ai-sdk#TimeoutConfig)
- [`provider.APICallError`](https://pkg.go.dev/github.com/grafana/ai-sdk/provider#APICallError)

---

← [Streaming over HTTP](streaming-http.md) · [Docs index](../README.md) · [Fallback and registry →](fallback-and-registry.md)
