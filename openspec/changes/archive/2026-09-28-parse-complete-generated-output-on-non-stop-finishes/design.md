## Context

`GenerateText` calls `generateConfig.toStreamConfig()` (`options.go`) and drains `streamTextWithConfig`, then copies final output and error into `GenerateTextResult`. `ToolLoopAgent.Generate` likewise drains `streamTextWithConfig`, but overrides the merged config to skip non-stop parsing (`agent.go`). At the final-step branch of `streamtext.go`, `ParseComplete` currently runs when `parseOutputOnNonStop` is true or the finish is `stop`. Generated calls set the flag false; streaming calls use true. This leaves valid generated non-stop text unparsed and suppresses validation errors.

The registered reference is Vercel `ai@7.0.109` at `4e8c387622ee1bb0d55841664416d38754d5c9a3`, not the issue's earlier `7.0.107` reference. In pinned `packages/ai/src/generate-text/generate-text.ts` (1590–1604), complete output is parsed if the last step finishes `stop`, OR finishes other than `tool-calls` with `text.length > 0`. Pinned `packages/ai/src/agent/tool-loop-agent.ts` (197–257) delegates `.generate()` to `generateText`. Pinned `output.ts`/`output.test.ts` validate complete object/array/choice/JSON responses. Go intentionally reports parse failure on the result (`OutputError`, `ErrNoObjectGenerated`) rather than turning it into the upstream throwing result property. The `structured-output` base spec and `TestGenerateText_OutputWithLengthFinishReason` currently assert stop-only generated parsing; streaming length parsing already has separate tests. `test/conformance/PARITY.md` calls both structured output and agent evidence `mixed` (core orchestration and agent entry point). The recorded OpenAI structured JSON length fixture covers streaming, not generated entry points.

## Goals / Non-Goals

**Goals:**
- Apply the pinned final-step parsing condition to `GenerateText` and `ToolLoopAgent.Generate` without changing `StreamText`/agent streaming final or partial/element behavior.
- Cover successful parse, parse/schema error, and skipped parse with retained final text, finish reason, and explicit typed accessor behavior.
- Amend the existing structured-output requirement; prove behavior with deterministic focused streams for both generated entry points.

**Non-Goals:**
- No changes to provider `DoGenerate`, provider adapters, SSE/UI chunks, streaming parse rules, partial snapshots/array elements, schema implementation, upstream pin, or public API.
- No legacy `generateObject`/`streamObject` or `repairText` additions. Do not edit recorded provider inputs or invent provider fixtures.

## Decisions

1. **Generated-operation-specific terminal predicate.** Model generated versus streaming completion explicitly in the stream configuration or final-step dispatch. For generated calls with configured `Output`, call `ParseComplete(finalStep.Text)` iff `finalStep.FinishReason.Unified == stop || (finalStep.FinishReason.Unified != tool-calls && len(finalStep.Text) > 0)`. This is based on the final accumulated step, not an earlier tool step; use raw text length (no trim), matching upstream. `stop` with empty text attempts parsing and reports a structured parse failure; `length`, `content-filter`, `error`, and `other` with empty text skip both output and error; `tool-calls` always skips, including nonempty text. Preserve StreamText's existing unconditional final parse when output is configured. **Alternative rejected:** flip the current boolean to true for generated calls; this would erroneously parse empty non-stop and `tool-calls` responses. **Alternative rejected:** modify `StreamText`'s shared predicate for every call; this would change existing streaming guarantees and the recorded length fixture.

2. **Preserve Go result projection.** Leave complete-parser implementations and `GenerateTextResult`/`output.Value[T]` signatures unchanged: parse failures populate `OutputError` (wrapping `ErrNoObjectGenerated`) and leave `Output` nil, not a top-level `GenerateText` error; skip leaves both nil while `output.Value[T]` reports missing output through an `output.OutputAccessor` wrapper of the generated result. `GenerateTextResult` has `Output`/`OutputError` fields, not `OutputValue()`/`OutputError()` methods; use the existing `output.ObjectResult[T]` wrapper or a test-local adapter for typed assertions, with no public API change. Retain `Text` and `FinishReason` in all outcomes. **Alternative rejected:** translate upstream throwing output-property access directly into a top-level call error, which breaks the explicit Go API contract.

3. **Focused parity evidence, not invented provider input.** Table-drive six finish reasons × three text conditions (valid, invalid, empty) for each of object, array, choice, JSON through both generated entry points; use non-null valid JSON, schema-invalid but parseable input for each schema-bearing mode, and malformed JSON for JSON mode. Assert parsed value versus `OutputError` versus skipped parse, as well as typed `output.Value[T]` through `output.ObjectResult[T]` or a test-local `OutputAccessor` adapter (never directly on `GenerateTextResult`), preserved text and finish. Update the existing length-finish generated assertion and retain streaming length regression and partial/array-element tests. Test synthetic provider streams in focused tests, not under `recorded/` or `upstream/`; no SSE changes imply no frontend-wire scenario or snapshot regeneration anticipated. **Alternative rejected:** treating the existing recorded OpenAI length fixture as proof of generated-call parity; it only covers its configured stream path and its original provider input cannot be modified.

## Risks / Trade-offs

- [Shared terminal parsing code can change streaming behavior accidentally] → Isolate the generated condition, verify `StreamText`/agent stream length finishes and partial/element delivery still pass, and keep recorded fixture unchanged.
- [Upstream throws on output access while Go uses explicit errors] → Keep existing public Go error contract; assert error wrapping and typed accessor instead of introducing a breaking API change.
- [A JSON `null` parse succeeds with nil `Output`/`OutputError` and `output.Value[any]` treats nil as missing] → Use non-null valid JSON in this finish-reason regression; do not silently change existing JSON-null accessor semantics as part of this fix.
- [Synthetic tests do not establish every provider's live response] → Classify this as focused core/agent evidence; run parity checks at implementation time and document any provider-boundary gap without manufacturing recorded inputs.

## Migration Plan

Implementation is internal to final generated-call completion. Existing callers retain the same result shape; non-stop generated responses with parseable text gain output or validation error. Rollback would revert the predicate and updated tests/spec together; no stored-data or wire migration.

## Open Questions

None for this scoped plan. The pinned agent implementation delegates generation to pinned `generateText`; the behavior contract above applies to the Go agent generated entry point separately from its streaming entry point.
