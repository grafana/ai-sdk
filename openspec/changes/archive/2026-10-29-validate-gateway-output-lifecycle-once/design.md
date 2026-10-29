# Design

## Context

- The archived change `2026-10-29-share-gateway-output-lifecycle-validation` (#609) made the runtime enforce every lifecycle rule. Its D8 (option A) kept a second, delegated evaluation inside ProviderWire V4:
  - `streamState` owns a `StreamValidator` seeded from `call.ToolHistory()` and a `phase` projection (`providerwire/v4/stream.go:254-272`);
  - `runStream` checks a first `stream-start` with it (`stream.go:288`), and `processStreamPart` checks every other part with it before dispatch, then projects `phase` (`stream.go:344-351`);
  - `mapUnarySuccess` calls `ValidateGenerateResult` with `unaryMappingContext.history` (`response.go:93-98`);
  - the handler reads `call.ToolHistory()` only to feed these two (`handler.go:93-105`).
- In production, the clean close already comes from the runtime: `consumeStreamParts` returns on `WaitFinished` and writes the terminal error for any other non-part wait (`stream.go:316-330`). Nothing outside the white-box tests reads `phase`.
- The runtime's guarantees that ProviderWire now relies on are these:
  - a delivered part has a LanguageModelV4 type;
  - only the first part can be a `stream-start`;
  - a `finish` has a non-nil `FinishReason` and `Usage`;
  - `Generate` never returns a nil result with a nil error (`runtime.go:129-139`).
- White-box ProviderWire assertions that depend on the second evaluation:
  - `response_test.go` `TestUnarySuccessValidation` rows: nil result, a provider-executed custom part, and an unregistered finish reason. `TestUnaryProviderToolResult_LargeAndPreliminary`: its preliminary result with no final;
  - `reasoning_test.go` `TestReasoningFrameEncodingAndLifecycle`: its second loop, which is all lifecycle;
  - `state.phase == segmentFinished` in `reasoning_test.go:75`, `metadata_test.go:89` and `generated_file_test.go:37`;
  - `function_tools_test.go` `TestRuntimeProviderToolHistory` and `TestUnresolvedProviderCalls_RejectsAmbiguousHistory`, through the test-only `unresolvedProviderCalls` in `tool_history_test.go:47`;
  - all of `lifecycle_agreement_test.go`.

  The other direct calls (`citations`, `generated_file`, `metadata`, `native_response`, `raw_usage`, `sources`, and the rest of `response` and `reasoning`) assert representation or content validity, or a success. Removing a check can only turn a rejection into an acceptance, so success assertions cannot break.
- The `execution` tables already cover most moved rules: `stream_validation_test.go` has the reasoning rules with `rejectAt` indices, `unary_validation_test.go` has nil, flags and unregistered reasons, and `history_test.go` has mismatched results and re-emission with another name.

## Goals / Non-Goals

**Goals:**
- ProviderWire's non-test code evaluates no lifecycle rule and holds no lifecycle state.
- Byte-identical ProviderWire responses, proven by tests that pass both before and after the removal.

**Non-Goals:**
- Unexporting the `execution` validators. They are package-internal to the `ai-gateway` module, and the runtime and its tests use them.
- Adding nil guards in ProviderWire for invariants the runtime guarantees.

## Decisions

### D1. ProviderWire trusts the runtime outcome

- `streamState` keeps `execution` and `includeRaw`. The validator, `phase`, `segmentPhase`, `segmentOpen` and `segmentFinished` are removed.
- `runStream` takes the real-start branch when `part.Type == provider.PartStreamStart`. The runtime delivers a `stream-start` only as the first part, so this is the same branch as before.
- `processStreamPart` dispatches directly. Its explicit `case provider.PartStreamStart` is removed. The `default` case still returns `streamPartAdapterFailure`, so any part type that ProviderWire cannot encode in mid-stream is still refused. That refusal is representation, not a lifecycle check, and the runtime never reaches it.
- The `finish` branch dereferences `FinishReason` and `Usage` as it does today. The runtime rejects a `finish` without them.
- **Alternative: keep the `stream-start` case and the nil guards.** Rejected: they restate runtime rules, which is what this change removes, and the `default` case already covers any surprise type.

### D2. Unary mapping drops history

- `mapUnarySuccess` keeps only its representation and content checks, in their current order. `unaryMappingContext` keeps `overview`.
- The handler's `result == nil ||` guard stays. It is ProviderWire's own guard against a nil it cannot map, and it never fires behind the runtime.
- The reject order changes from lifecycle-then-representation to representation only. It cannot be observed: every rejection writes the same 500 document. The moved `TestLifecycleAgreement_UnaryDocuments` pins this.

### D3. Remove `Call.ToolHistory`

- After D1 and D2, its only callers are an `execution` test (`history_test.go:96-108`, "valid history is exposed as a copy"). The runtime seeds its own validators from `c.history`.
- The accessor is removed, and so is that subtest. `ToolHistory.clone`, which only the accessor used, is removed too, along with its test. Each validator already copies history into its own state. The "History is not shared" scenario stays covered by `TestStreamValidator_HistoryIsNotShared`.
- **Alternative: keep it for a future adapter.** Rejected (YAGNI): the modified spec says adapters do not evaluate lifecycle rules, so an accessor would only invite a second evaluation again.

### D4. Where each moved assertion lands

- **Through the runtime, in ProviderWire:** a new `providerwire/v4/invalid_output_test.go` replaces `lifecycle_agreement_test.go`:
  - **Stream table.** It keeps every stream case, each with a hard-coded `rejectAt` (or -1) taken from today's agreed verdicts. A case's history becomes a prompt with the pending provider-executed call. Each case is served through `newRuntimeHarness`:
    - a rejected case's body must equal the body for `parts[:rejectAt]` followed by a part that ProviderWire refuses as content (a nil source);
    - an accepted case must end without a terminal error.

    Any rejection at the same index writes the same bytes, whichever layer rejects. Equality therefore proves that ProviderWire stops at the runtime's part and writes the frames it wrote before.
  - **Unary table.** It keeps every unary case with a hard-coded verdict. A rejected case must write the fixed 500 internal document with the selected-attempt metadata, and an accepted one must write 200.
  - `TestLifecycleAgreement_StreamFrames` and `TestLifecycleAgreement_UnaryDocuments` move unchanged, under names that drop "Agreement".
- **In the `execution` rule tables:** rows are added only where a moved assertion is missing:
  - stream: a reasoning start that repeats a still-active ID (`reasoning_test.go` case 3);
  - unary: a provider-executed custom part (`response_test.go`), if no provider-executed non-tool row covers it;
  - history: two client calls with the same ID, and a client call whose result names another tool (`function_tools_test.go:229`), since today's rows use provider-executed calls.

  The table was checked against these when this design was written. Implementation re-checks it before adding a row.
- **Phase assertions** are dropped. `StreamValidator.SegmentFinished` and `WaitFinished` are covered by `stream_validation_test.go` and `runtime_test.go`.
- **History inspection:** `TestRuntimeProviderToolHistory` calls `execution.RebuildToolHistory(opts.Prompt)` directly. It tests that ProviderWire's request mapping preserves what the runtime reads, not a ProviderWire rule. Its HTTP-status rows stay. `TestUnresolvedProviderCalls_RejectsAmbiguousHistory` is deleted after its two prompts are confirmed in `history_test.go`.
- **Alternative: compute the expected index with `execution.NewStreamValidator` in the ProviderWire test.** Rejected: hard-coded indices keep the ProviderWire test independent of the code that decides the rejection.

### D5. Commit order proves byte identity

1. **`test(ai-gateway): cover invalid output through the ProviderWire handler`.** Adds `invalid_output_test.go` and the missing `execution` rows while ProviderWire still double-checks. Everything passes on the pre-change code.
2. **`refactor(ai-gateway): validate provider output lifecycle once`.** Applies D1-D3, edits the white-box tests, deletes `lifecycle_agreement_test.go` and the wrapper, and updates the godoc. The tests from commit 1 pass unchanged. That is the byte-identity evidence, together with the unchanged handler suites (`stream_test`, `stream_segments_test`, `stream_segments_replay_test`) and `TestTestAdapter_MatchesProviderWire`.
3. **`docs(openspec): archive validate-gateway-output-lifecycle-once`.**

### D6. No upstream parity change

This is a Gateway-internal refactor, which counts as a parity-preserving Go adaptation. There is no `@ai-sdk/*` counterpart at the registered baseline, and the LanguageModelV4 stream-part contract is unchanged. `upstream.yaml`, `knownGaps`, the fixtures, the expectations and the PARITY.md row stay unchanged.

## Risks / Trade-offs

- [A runtime regression would now reach ProviderWire unchecked, for example as a nil `FinishReason` panic] → The runtime's rule tables, runtime outcome tests and the new handler tables gate every change to `execution`. This is the single-owner design #394 intends.
- [Hard-coded `rejectAt` indices can drift from a future rule change] → That is intended: a rule change must update the indices explicitly, alongside the `execution` table.
- [The reference stream's nil source might itself be rejected by the runtime after a `finish`] → That is harmless: it is still rejected at the same index, with the same frames.

## Migration Plan

No operator or wire migration. Rollback is reverting the PR.
