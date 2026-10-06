## 1. Reproduce the leak

- [x] 1.1 Run the issue's probe shape against `43527ef`: after abandoning `FullStream`, the run goroutine sits in `emit` at `[chan send]` before and after `cancel()`, and `Wait()` is still blocked.
- [x] 1.2 Confirm `emit` is the only send site for `r.fullStream`, that only the run goroutine calls it, and that `losslessStream.send` cannot block on a reader.
- [x] 1.3 Confirm `run` derives its own context for the total, step, first-chunk and chunk timeouts, so the fix has to watch that context and not only the caller's.

## 2. Establish a failing regression

- [x] 2.1 Add `manyTextDeltaParts`, a provider stream of more deltas than the buffer holds that stops on context cancellation.
- [x] 2.2 Add `TestStreamText_CancelFinishesAbandonedFullStream` with two cases, a caller cancel and a one-second total timeout: read one part, wait for `len(fullStream) == cap(fullStream)`, abort, then require `Wait()` to return within ten seconds and drain the closed channel.
- [x] 2.3 Run it against the unfixed `emit`: both cases fail with `Wait() is still blocked after the run was aborted` after the full ten seconds. An earlier version of the test cancelled immediately and passed either way, which is why it waits for the buffer to fill.
- [x] 2.4 Run it against an `emit` that watches only the caller's context: the caller case passes and the total-timeout case fails, which is why `run` publishes its derived context.

## 3. Release the run goroutine

- [x] 3.1 In `run`, after deriving the timeout and operation contexts and before the first emit, store `ctx.Done()` on the result as `ctxDone`.
- [x] 3.2 Make `emit` attempt a non-blocking send, then wait on the send or `ctxDone`, with a comment explaining why the order matters.

## 4. Validate

- [x] 4.1 `go test -race ./ -count=1`: ok, including the existing abort, finish and cancellation tests that depend on post-cancellation delivery.
- [x] 4.2 `mise run test-conformance`: PASS.
- [x] 4.3 `mise run fmt-check`, `mise run vet`, `mise run lint`: clean.
- [x] 4.4 `openspec validate --all --strict`: this change validates; the two failing items are pre-existing main specs it does not touch.
