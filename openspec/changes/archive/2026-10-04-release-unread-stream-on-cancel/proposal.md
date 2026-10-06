## Why

[Issue #165](https://github.com/grafana/ai-sdk/issues/165) reports a goroutine leak with no escape hatch. `emit` sent on the `FullStream` channel with no cancellation case, so a consumer that stopped reading parked the run goroutine once the 256-part buffer filled. `run` never returned, `r.done` never closed, and `Wait`, `Text`, `Steps` and `ToolResults` blocked for the life of the process. Cancelling the context did not help, even though [`docs/guides/streaming-http.md`](../../../docs/guides/streaming-http.md) tells a consumer to "drain the stream or cancel its context".

A probe confirms it: after abandoning `FullStream` the run goroutine sits in `emit` at `[chan send]`, stays there a second after `cancel()`, and `Wait()` is still blocked two seconds later.

## What Changes

- `emit` tries a non-blocking send first, so every part that fits is delivered as before.
- When the buffer is full, `emit` waits on the send or on the run context's cancellation, so a run cancelled by the caller or aborted by a configured timeout drops the part, returns, closes `FullStream` and releases `Wait` and the blocking accessors.
- Add a regression test that abandons a stream mid-step, waits for the buffer to fill, then either cancels or lets a total timeout expire, and requires `Wait()` to return.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `stream-text-lifecycle`: Add the rule that cancellation, by the caller or a configured timeout, releases a stream nobody is reading, while a reading consumer still receives parts that fit after cancellation.

## Impact

The change is confined to `streamtext.go` (one struct field, its assignment, and `emit`) and one test. No public API, dependency, baseline or conformance change. A run cancelled while its buffer is full now loses the parts that did not fit, which is the behavior that lets it finish; a consumer still reading loses nothing, and every existing abort and finish test passes unchanged.
