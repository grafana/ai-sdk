## Context

The `fallback.Model` wraps multiple `provider.LanguageModel` candidates and tries them in order. For `DoGenerate`, errors are returned synchronously and fallback works correctly. For `DoStream`, the contract is `(*StreamResult, error)` where `StreamResult` contains a `<-chan StreamPart`. Some providers (Anthropic) return `(result, nil)` immediately and defer HTTP-level errors to the channel as `PartError` events. The current `DoStream` only checks the synchronous error, so async errors bypass fallback entirely.

## Goals / Non-Goals

**Goals:**
- Detect leading stream errors (first chunk is `PartError`) and trigger fallback to the next candidate
- Apply the existing `decider` function to stream errors for consistent policy
- Work for any provider regardless of error reporting pattern (sync or async)

**Non-Goals:**
- Detecting mid-stream errors (after valid data has been emitted) -- these cannot trigger fallback since data has already been consumed
- Changing the `LanguageModel` interface or provider implementations
- Adding retry logic (fallback is try-next, not retry-same)

## Decisions

### Decision: Peek at first chunk in fallback layer

The fix lives entirely in `fallback.DoStream`. After calling `c.DoStream()` and getting `(result, nil)`, read the first chunk from the stream channel:

- **PartError**: Treat as synchronous error. Drain remaining stream in a goroutine (prevent leaks), apply the decider, try next candidate.
- **Valid chunk**: Wrap the stream in a new channel that replays the peeked chunk first, then forwards remaining chunks.
- **Closed channel** (empty stream): Return as-is with an empty closed channel.

**Alternatives considered:**
- *Eager read in each provider*: Would require changes to every provider and changes `DoStream` blocking semantics for all callers, not just fallback.
- *Synchronous HTTP error detection in Anthropic*: Depends on SDK internals, only fixes one provider, and some errors may still arrive async.

The fallback layer is the right place because it owns the fallback decision and needs to be resilient to different provider patterns.

### Decision: Goroutine + channel for stream wrapping

When the first chunk is valid, create a new buffered channel and a goroutine that sends the peeked chunk then forwards the rest. This preserves the existing channel-based streaming contract without requiring changes to `StreamResult`.

The wrapper goroutine is lightweight -- it just relays from one channel to another with one extra send at the head.

### Decision: Goroutine drain for failed streams

When a `PartError` is detected on the first chunk, the remaining stream must be drained to avoid leaking the producer goroutine. A `go func() { for range stream {} }()` handles this without blocking the fallback loop.

## Risks / Trade-offs

- **[Latency]** `DoStream` now blocks until the first chunk arrives instead of returning immediately. For error cases this is near-instant. For healthy streams the first chunk arrives within the normal streaming latency. This is acceptable for the fallback use case. → Mitigation: Only affects calls through `fallback.Model`, not direct provider usage.
- **[Extra goroutine per successful call]** The stream wrapper adds one forwarding goroutine. → Mitigation: The goroutine is trivial (channel relay) and bounded by stream lifetime. This matches existing patterns in the codebase.
- **[Only catches leading errors]** Errors that appear after valid data (e.g., mid-stream disconnect) won't trigger fallback. → Mitigation: This is by design -- mid-stream fallback would require buffering all data and replaying it, which is a fundamentally different problem. Leading errors cover the main use case (bad model ID, auth failure, rate limit).
