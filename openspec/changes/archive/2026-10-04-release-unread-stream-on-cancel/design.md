## Context

At `43527ef`, `StreamTextResult.emit` was `r.fullStream <- part` with no other case, and it is the only send site for the channel (`streamtext.go:2011-2013`). `run` closes `r.fullStream` and `r.done` through defers (`streamtext.go:420-422`), so a parked `emit` keeps both open. `Wait` is `<-r.done` (`streamtext.go:2474`), which is why every blocking accessor hangs. The auxiliary streams are not affected: `losslessStream.send` queues under a mutex and never blocks on a reader.

Upstream TypeScript has no equivalent: a consumer that stops reading a web `ReadableStream` applies backpressure that the producer resolves through cancellation of the stream itself, so there is no registered behavior to port. This is a Go-specific defect in a Go-specific mechanism, so the fix follows the Go contract the docs already state.

## Goals / Non-Goals

**Goals:** Let a cancelled run finish when nobody is reading; keep ordinary delivery and the existing abort and finish parts intact for a consumer that is reading; keep the fix at the single choke point every emitter already routes through.

**Non-Goals:** Changing the buffer size, adding a drop counter or a new public signal, reworking `losslessStream`, and the separate ordering issue #166.

## Decisions

### 1. The run context's cancellation signal, stored on the result

`emit` has no context parameter. Only the run goroutine calls it: concurrent tools hand their events to `runToolsEmitOnCompletion` over a channel, and that loop, which waits for every tool to report before returning, is what emits. Threading a context through every emitter would be a large diff for one channel send, so `run` stores `ctx.Done()` on the result before its first emit.

The channel comes from the context `run` derives, not the caller's. `run` wraps the caller's context in `WithTimeout` for the total timeout and then in `WithCancel`, whose cancel function the step, first-chunk and chunk timers call. Watching only the caller's context left every timeout-driven abort of an abandoned stream deadlocked. A `nil` channel from a context that cannot be cancelled blocks forever in select, which is today's behavior for that case.

### 2. Non-blocking attempt first

A plain `select` over the send and the cancellation channel picks at random when both are ready, so a cancelled run with a reading consumer would drop roughly half its remaining parts, including `abort` and `finish`. Trying the send alone first means a part is dropped only when the buffer is full, which is the state that defines an abandoned stream.

### 3. Dropping rather than draining internally

An internal drain would keep the run alive and discard everything, which loses the parts a slow consumer would still have read. Dropping only once cancellation and a full buffer coincide keeps the lossless path for every other case.

## Risks / Trade-offs

A consumer that cancels its context and then resumes reading can observe a gap. That is inherent to cancelling: the alternative is the current deadlock. `FullStream` still closes, so the consumer sees the end of the stream rather than a stall.
