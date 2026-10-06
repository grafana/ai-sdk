## 1. Reproduce and lock the streaming contract

- [x] 1.1 Add synthetic interleaved-ID cases to `TestExtractReasoning_Stream` in `middleware/extract_reasoning_test.go`: starts `a,b`, interleaved plain text and split nonempty reasoning tags, reversed ends, a first-empty/reasoning-only block and multiple nonempty segments in one block; assert exact emitted part order, text start/delta/end ID matching, unique stream-global reasoning IDs, and paired reasoning starts/ends for nonempty and first-empty segments. Run `go test ./middleware -run TestExtractReasoning_Stream` and verify the new cases fail before the fix.
- [x] 1.2 Add a provider-independent StreamText/UI regression (root stream test or `test/conformance/ui/` fixture) with the same overlapping input; assert UI chunk ID lifecycles and assembled text/reasoning content, without writing synthetic provider `recorded/` inputs.

## 2. Correct state ownership

- [x] 2.1 Change `extractionState` in `middleware/extract_reasoning.go` to retain delayed text starts by provider text ID; flush/delete only that ID before its first visible delta or matching end. Preserve existing tag parsing, per-ID buffering and single-block behavior.
- [x] 2.2 Move reasoning ID allocation to stream scope while retaining each per-ID segment's active ID across interleaved deltas and its end; confirm no collision with another block, later nonempty segment or first empty segment. Preserve the pinned upstream behavior for a later empty segment after prior nonempty reasoning (end without a new start); do not add a new start guarantee.

## 3. Frontend and parity verification

- [x] 3.1 Add a deterministic overlapping extraction scenario under `test/integration/testserver/` and a matching Vitest test under `test/integration/`; use `parseJsonEventStream` with `uiMessageChunkSchema`, assert text/reasoning chunk ID pairing and order, and verify `readUIMessageStream` assembles separate correct parts.
- [x] 3.2 Run `go test ./middleware -run TestExtractReasoning_Stream`, relevant root StreamText/UI tests, `mise run test-integration`, and `mise run parity-check`; update only provider-independent UI expectations if needed and verify all touched fixture provenance. Check existing single-ID cases, first-empty reasoning and split-tag behavior for regressions.
- [x] 3.3 Verify cancellation with `go test ./middleware -run 'TestTransformStream/ContextCancellation'` and a focused `ExtractReasoning` middleware test using a controlled source blocked after overlapping IDs have pending delayed starts. Account for parts already emitted or buffered before cancellation; cancel while the source is quiescent and assert prompt transformed-stream closure without a cancellation-triggered flush of those pending starts. Do not require zero post-cancellation parts in all races or change `TransformStream` cancellation semantics.
