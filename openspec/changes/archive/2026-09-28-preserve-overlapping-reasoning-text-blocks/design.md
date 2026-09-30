## Context

The streaming wrapper in `middleware/extract_reasoning.go` already buffers tag parsing per text ID (`extractions`), but stores only one delayed text start for the whole stream and increments a reasoning ID counter per text ID. For starts `a`, `b` followed by interleaved deltas, the start for `a` is overwritten, a delta for `a` can be preceded by the start for `b`, and both blocks can emit `reasoning-0`. The registered upstream `ai@7.0.109` at `4e8c387622ee1bb0d55841664416d38754d5c9a3` instead keys delayed starts by text ID and owns the reasoning ID counter at stream scope; its overlapping-block test asserts combined text and reasoning content. This is a Go implementation bug, not a desired deviation. The delta spec adds event-lifecycle and ID requirements beyond the upstream test's aggregate assertions.

## Goals / Non-Goals

**Goals:**
- Keep text start/delta/end events correctly associated with their provider text ID when blocks overlap, delaying starts for reasoning-first content as before.
- Keep each reasoning segment's start, deltas and end on one ID and make IDs unique across segments of the same stream.
- Prove both provider stream events and resulting schema-parsed UI chunks / message assembly using deterministic synthetic model streams.

**Non-Goals:**
- Change generate extraction, middleware public options, provider API, or serialized UI chunk types.
- Invent provider recordings or reorder independent provider events to group blocks.

## Decisions

- Replace the single delayed start with per-text-ID pending starts in `extractionState`, flushing/removing only the matching ID on visible text or on its text end. Keep the existing sequential transform and per-ID tag buffers; unlike a global queue, an ID-keyed lookup cannot consume another block's start. Preserve emission of a text start and end for a reasoning-only block.
- Own the reasoning ID allocator at stream scope rather than per block, while keeping the active segment ID in per-ID extraction state. Allocate lazily when a segment emits a start, delta, or end; reuse the ID through that segment's end and clear it afterward. Nonempty segments have paired starts/ends; retain the existing start/end pair for a block's first empty reasoning segment. Pinned upstream `ai@7.0.109` (`extract-reasoning-middleware.ts:230-247`) emits an end without a start for a later empty segment after nonempty reasoning, so do not introduce a new start guarantee for that case or classify it as a parity deviation. A stream-global allocator avoids collisions; per-ID active IDs support simultaneously open segments. Keep current tag splitting, separator, and reasoning-first handling unchanged.
- Exercise synthetic interleaved starts, interleaved split tags and nonempty reasoning deltas, reversed text ends, a first-empty/reasoning-only segment and multiple nonempty segments within one block in `middleware/extract_reasoning_test.go`; assert ordered emitted parts and ID pairing for nonempty and first-empty segments, not just concatenated content. Follow with a StreamText UI chunk/assembled-message regression using the established root test or provider-independent `test/conformance/ui/` pattern; add a dedicated Go testserver scenario and Vitest test using `parseJsonEventStream`, `uiMessageChunkSchema` and `readUIMessageStream` as in `test/integration/reused-part-ids.test.ts`. These do not claim live provider evidence. No provider input fixture is modified.

## Risks / Trade-offs

- [A pending start could be flushed for the wrong block during a tag transition or text end] → Assert exact text start/delta/end IDs and ordering with interleaved ends and reasoning-only input.
- [Split tags or interleaved nonempty reasoning could allocate two IDs or end on another segment's ID] → Assert ID pairing across split tags, first-empty reasoning and overlapping nonempty segments; retain single-ID regression coverage.
- [Stream parts look correct but UI assembly conflates blocks] → Assert parsed UI chunks and final assembled text/reasoning parts in cross-language integration, and run parity checks if expectations change.

## Migration Plan

No migration or public API change; deploy as a streaming bug fix. Revert this change if an unanticipated provider event-order regression occurs.

## Open Questions

None for the proposal. The exact event-order assertions should follow the registered upstream semantics while preserving existing single-block behavior.
