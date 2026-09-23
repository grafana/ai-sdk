## Context

The upgrade advances `ai@7.0.65` to the nine-package reference frozen in
[selected-target.json](selected-target.json). Per-package source commits are
authoritative; the shared upstream checkout is not switched. Coverage and
remaining work belong in `test/conformance/PARITY.md`, not this design.

## Goals / Non-Goals

**Goals:** validate one frozen baseline transition, account for all supported surfaces, and register remaining behavioral work independently.

**Non-goals:** full feature parity, adoption of unsupported product families, fabricated provider inputs, or publication based only on workspace builds.

## Decisions

- Retain the selector record with the change rather than reselection or a hand-built package set. Per-package commits are authoritative, not a single `ai` tag.
- Apply candidate pins and regenerate existing expectations before compatibility fixes. Replayed failures establish the actual upgrade regression contract; source/tests explain each change.
- Use `PARITY.md` for concise surface dispositions and linked issues for actionable work. Release deltas and existing gap inventories are investigation leads, not the assessment boundary.
- Separate changed behavior from missing evidence. Recorded/imported provider inputs remain untouched; synthetic failures belong in focused tests.
- Keep public module ordering explicit. Consumers adopt only merged, publicly resolvable producers. The owner explicitly approved delivering the existing Mantle assistant-history correction in two stages: this upgrade publishes the OpenAI correction; a separate consumer change then repins Bedrock and adds the preserved regression. This is a scoped delivery decision for an existing defect, not a claim of Mantle parity or a relaxation of GOWORK=off checks.
- Publish only after bidirectional parity review, specification completion, required checks, and issue-label verification. Do not treat absent verification metadata as permission to weaken validation.
- Preserve all upstream streaming inputs byte-for-byte. The new parallel-wrapper fixture has malformed first/last JSON events in the selected source. Retain those errors and decode subsequent valid events using the official SDK's SSE framing decoder plus event-local JSON decoding; keep official SDK request/authentication and initial HTTP/transport error handling. Unlike the typed SDK iterator, one malformed JSON event must not terminate decoding or trigger setup retries.
- Recognize internal parallel wrappers only for undeclared `parallel` calls whose recipients are all declared function tools. Preserve wrapper metadata for scalar stateful continuation, fall back atomically for invalid wrappers, and leave the separately registered existing multipart conversion limitations visible (#221).
- Include the finish-reason gate (#208): recoverable malformed data newly exposes valid local tool calls after an error. Dispatch only on stop/tool-calls, retain approval observations, and require matching client results/denials for continuation. Do not hide recovered calls or broaden the Mantle exception.
- Acquire streaming responses through the existing SDK endpoint with its raw-body destination option; one SDK framing decoder owns construction and closure. Retain finish state/usage until EOF so buffered wrappers precede finish and late malformed data cannot overwrite errors with success.
- Keep text/reasoning ID reservations invocation-wide but active mappings step-local. The two content families have separate namespaces; generating a colliding ID uses a numeric suffix rather than another generator call.

## Risks / Trade-offs

- Broad assessment exceeds fixture coverage → inspect exact source/tests by supported surface, explicitly retain unresolved questions, and do not claim completeness prematurely.
- Gateway schema/witness drift can hide integration failures → exercise registered-client contract and real-command tests before re-attestation.
- Workspace builds can mask unpublished dependencies → run the isolated public-module gate and inspect actual consumer versions.
- This upgrade is stacked on workflow PR #205; validate against that base. Changes inherited from main, including #194, are not new upgrade implementation.

## Migration Plan

Apply the approved record to every parity consumer, install the locked dependency set, regenerate expectations, resolve compatibility blockers, then validate and record completed verification. No production deployment is part of this change. If blocked, preserve the candidate and evidence without setting a successful verification date or publishing a completed upgrade.

## Open Questions

PARITY.md records remaining API questions and scoped packages, including UI outcome hooks versus #181, raw SDK constructor URL policy, and optional caller/Agent preparation interfaces. These are not implicit API approvals. The specifically approved Mantle delivery split is #207; no unpublished Go requirement or production replacement is introduced.
