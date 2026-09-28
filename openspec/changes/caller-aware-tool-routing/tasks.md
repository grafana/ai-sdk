## 1. Pin parity evidence and tests

- [x] 1.1 Compare `ai@7.0.109` caller configuration, `streamText`/`generateText` execution, approval resume, and streamed-output tests against the Go paths; note Go-specific adaptations and upstream limits in the implementation review.
- [x] 1.2 Add focused failing tests for caller mapping validation, active-tool filtering, model/execution set separation, local binding, announcement deduplication, and direct/provider visibility.
- [x] 1.3 Add focused failing provider-call capture tests for manual `allowedCallers`, provider callback preparation and direct+provider routing when the callback returns replacement options.

## 2. Caller routing implementation

- [x] 2.1 Add opt-in caller definitions and mapping options to the root API, preserving existing no-configuration behavior.
- [x] 2.2 Resolve and validate callers once, then prepare per-step active model/execution sets and caller messages without SDK-owned mutation of caller-owned tools/options.
- [x] 2.3 Route provider request tools through the model set and local call parsing/approval/execution through the execution set; keep pre-step approval resume aligned with upstream's original tool set.
- [x] 2.4 Verify dynamic and provider-executed calls, per-step overrides, named tool choice, `GenerateText`, and Agent delegation against the new routing tests.

## 3. Streaming result tests and implementation

- [x] 3.1 Add failing tests for single-result compatibility, multiple preliminaries and repeated final, empty stream, mid-stream error/cancellation, output conversion, concurrent interleaving and final-only continuation.
- [x] 3.2 Add an opt-in Go streaming execution interface and adapt local execution and serialized event delivery without concurrent `OnChunk` callbacks or premature final results.
- [x] 3.3 Support preliminary/final delivery for both automatically approved and resumed approved local tools; verify denied/pending tools never run and provider-executed tools do not run locally.
- [x] 3.4 Add a provider-independent UI conformance fixture and a schema-parsed frontend integration scenario for local preliminary/final chunks; do not synthesize recorded provider input.

## 4. Finish and verify

- [x] 4.1 Update godoc and the relevant `docs/` guide to describe caller configuration, manual provider options, streamed execution, and upstream's non-authorization boundary.
- [x] 4.2 Run focused Go tests, `mise run test-integration`, `mise run parity-check`, and relevant formatting/vet/lint checks; review the full diff against the registered upstream source and fixture provenance.
