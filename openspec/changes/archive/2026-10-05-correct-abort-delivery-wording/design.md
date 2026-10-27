## Context

`StreamTextResult.abort` at `0e8f35e` emits `StreamAbort` and returns; `run` then closes `FullStream`. `TestStreamTextContextCancellation` and the three `stream-abort` UI conformance fixtures already encode `abort` with no `finish`. Upstream `packages/ai/src/generate-text/stream-text.ts` at `ai@7.0.116` behaves the same way.

## Decisions

Correct the requirement, not the code. Adding a `finish` after `abort` would contradict the upstream baseline, the `conformance-testing` requirement that a canceled stream ends with exactly one `abort` chunk, and the committed fixtures. The archived #345 change stays as the record of what was proposed; this change carries the correction.
