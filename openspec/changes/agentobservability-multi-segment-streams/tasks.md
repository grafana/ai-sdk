# Tasks

## 1. StreamRecorder multi-segment state (`middleware/agentobservability/map_stream.go`)

- [ ] 1.1 Write the `map_stream_test.go` tests first and run `cd middleware/agentobservability && go test -run 'TestStreamRecorder_MultiSegment' ./...` on `main`. Use synthetic parts only; do not invent provider fixtures.
  - `TestStreamRecorder_MultiSegment_ReusedIDsAppendSeparateParts` (fail-first: `main` merges reused IDs):
    - two segments each emit text and reasoning with ID `"0"` and end in `PartFinish`;
    - output is the segment-1 parts followed by separate segment-2 parts, in observed order;
    - a tool call in segment 1 with its result in segment 2 still pairs.
  - `TestStreamRecorder_MultiSegment_UsageKeepsStrongest` (regression pin: passes on `main` and after the change, because `StreamRecorder` already observes every part):
    - two finishes, 7/2 then 4/9, plus a `PartResponseMeta` with usage in segment 2;
    - expect input 7 and output 9, never summed.
  - `TestStreamRecorder_MultiSegment_LastFinishAndIdentity` (regression pin: passes on `main` and after the change):
    - the last finish reason maps to `StopReason`;
    - the last non-empty response ID and model win.
- [ ] 1.2 Implement design D4 and verify that 1.1 and the existing `TestStreamRecorder_*` tests pass (`go test ./...` in the module).
  - `PartFinish` replaces the `texts`/`reasonings` maps.
  - `streamOutputRef` holds the text or reasoning accumulator pointer, and `buildOutput` reads through it instead of `r.texts[ref.id]`/`r.reasonings[ref.id]`.
  - Tool accumulators stay keyed for the whole stream.
  - Update the `StreamRecorder` godoc for segment handling.
- [ ] 1.3 Extend `TestStreamRecorder_UsageAggregatesEveryPart`, or add a table case next to it, to cover a multi-finish stream whose later finish reports lower counters. It must keep the strongest values (the spec scenario "Multi-finish usage keeps strongest values"). Verify with `go test -run TestStreamRecorder_Usage ./...`.

## 2. Recording finalization (`middleware/agentobservability/recording.go`)

- [ ] 2.1 Write `recording_test.go` tests that use `testkit.NewEnv` and wait for `OnRecordComplete` before `Shutdown`. Run them on `main`: fail-first tests must fail, and regression pins must pass before and after the change.
  - `TestRecordingMiddleware_StreamMultiSegment_OneGeneration` (fail-first): two segments, then close. Expect:
    - exactly one export;
    - strongest usage and the second finish's stop reason;
    - output from both segments;
    - `completed_at` not before the observation of the second finish;
    - first-token time from segment 1, read from the env metrics or spans (time-to-first-token), with a measurable gap between segments.
  - `TestRecordingMiddleware_StreamStallAfterFinish_RecordsSnapshot` (regression pin: `main` ends at the finish): one segment, then the upstream blocks until the context is canceled. Expect:
    - no `call_error`;
    - the first segment's usage and finish reason;
    - `completed_at` at the finish, before the cancel. Use a gap the test can measure.
  - `TestRecordingMiddleware_StreamErrorAfterFinish_RecordsError` (fail-first): finish, then `PartError`, then cancel before close. Expect `call_error` to carry the stream error, not the cancellation.
  - `TestRecordingMiddleware_StreamErrorBeforeFinishThenCancel_RecordsError` (regression pin: `main` applies `CallError()` at the first finish): `PartError`, then `PartFinish`, then cancel before close. Expect `call_error` to be the stream error, not a cancellation, with the snapshot's usage and finish reason.
  - `TestRecordingMiddleware_StreamErrorAfterOnlyFinishThenClose_RecordsError` (fail-first; new behavior versus `main`, which stops observing at the finish): one `PartFinish`, then `PartError`, then a normal close. Expect `call_error` to be the stream error.
  - `TestRecordingMiddleware_StreamCloseAfterContextEnded_RecordsSnapshot` (regression pin, outcome only): finish, then the context ends, then upstream closes (`canceled = ctx.Err() != nil`). Expect the snapshot outcome with no `call_error`. The test cannot force this branch, because `select` picks randomly between the close and `ctx.Done`; both give the same outcome.
  - `TestRecordingMiddleware_StreamConsumerStopsReadingWithoutCancel`:
    - first case (regression pin): the consumer stops after the finish without canceling, and upstream then closes. The recording completes, and the `OnRecordComplete` signal is the wait condition.
    - second case (fail-first): upstream stays open. Assert that no generation is exported before the cancel (`main` exports at the finish), and that it is exported after the cancel.
  - `End` panic on close and snapshot paths (regression pin; `main` already recovers in `finalize`): extend `TestRecordingMiddleware_FinalizePanic` in `stream_panic_test.go`, or add a sibling, with a multi-finish stream on both the close path and the finish-then-cancel snapshot path. The panic comes from `panicTracer` span `End`, reached only through `finalize`/`endRecording`, so this does not cover snapshot capture (its guard is defensive and untested, design D1). The stream passes through unchanged, and the goroutine does not crash.
- [ ] 2.2 Adapt `TestRecordingMiddleware_StreamFinishIsRecordedBeforeDownstreamCancels`, renaming it to `TestRecordingMiddleware_StreamStopAtFinishThenCancelRecordsSuccess`, at drain timeouts 0 and 10ms:
  - the consumer breaks at finish and cancels;
  - the test waits for `OnRecordComplete` before `Shutdown`;
  - it asserts one generation with no `call_error`.

  Keep `TestRecordingMiddleware_StreamConsumerAbandons_RecordsCancellation`, `TestRecordingMiddleware_StreamCancellationFinalizesIdleProvider` and `TestRecordingMiddleware_StreamCancellationTakesPrecedenceOverPartError` unchanged. These cover cancellation before any finish, and they must still pass.
- [ ] 2.3 Implement design D1 in `runStreamTee` and verify with `cd middleware/agentobservability && go test -race ./...`. All 2.1/2.2 tests, `TestRecordingMiddleware_FinalizePanic`, `TestRecordingMiddleware_StreamSuccess_RecordsAccumulated`, the usage-export tests and the conformance tests must pass unchanged.
  - Remove the `finished` gate.
  - At each `PartFinish`, capture a stream-derived snapshot (D1) under `streamrecover.Recover`: the built output, usage, finish reason, response identity, `CallError()`, `FirstChunkAt()` and the finish time. Do not call `Generation()` per finish. Mark the snapshot invalid on panic.
  - Track a `PartError` with non-nil `APICallError` seen since the last finish (the same condition that updates `callError`).
  - In `finalize`, compose prompt-derived fields once, apply the D1 call-error rule and table (falling back to current state with `streamRec.CallError()` for an invalid snapshot), filter once, and keep `End` before `close(tee)`.
  - Rewrite the deferred comment and the `RecordingMiddleware` godoc to describe the new timing contract and when to wait for `OnRecordComplete`.
- [ ] 2.4 Update `docs/middleware/agent-observability.md` "Record generation relationships". Verify with `mise run lint-docs`. Describe:
  - one generation for streams with several finishes;
  - strongest-value usage and its multi-segment understatement;
  - the last finish reason;
  - draining to EOF or waiting for `OnRecordComplete` before closing the client.

## 3. Dependency check

- [ ] 3.1 Re-run `go list -m -versions github.com/grafana/agento11y/go github.com/grafana/agento11y/go-providers/anthropic`.
  - If a release newer than v0.19.0 exists, bump both together in `middleware/agentobservability/go.mod` and the agento11y require in `ai-gateway/go.mod`, run `mise run tidy`, and run `mise run test-agent-observability-middleware` and `mise run test-ai-gateway`.
  - Otherwise record "no newer release" in the PR description.
  - Never hand-edit the `middleware/agentobservability` version or changelog.

## 4. Gateway follow-up (after #583 merges; rebase first)

- [ ] 4.1 Rebase onto `main` containing #583 and confirm that `TestModelObservationChain_SequentialSegments` exists in `ai-gateway/cmd/grafana-ai-gateway/internal/service/model_observability_test.go`. Update its generation assertions and verify with `cd ai-gateway && go test -run TestModelObservationChain_SequentialSegments ./cmd/grafana-ai-gateway/internal/service/...`.
  - Normal close: input 7, output 9, `stop_reason` "stop".
  - Deadline and idle stall cases: 7/2, "tool_calls", no `call_error`.
  - Log and metric assertions stay unchanged.
  - Add an assertion: a stream error precedes a later finish, then a stall after that finish. The generation keeps the provider-error classification as its call error.
- [ ] 4.2 Re-express `specs/gateway-text-observability/spec.md` in this change against the merged spec, and verify with `openspec validate agentobservability-multi-segment-streams --strict`.
  - REMOVED "Agent Observability first-segment limitation", with Reason and Migration.
  - MODIFIED "Logical text lifecycle semantics": drop the "until #582" clauses from the scenarios "Usage is split across stream parts" and "Stream with several response segments".
  - MODIFIED "Logical finalization precedence and strongest usage": drop the scenario "Agent Observability stops at the first finish", and keep this change's after-finish exception.
  - Keep the ADDED requirement only where the merged text does not already cover it.
- [ ] 4.3 Remove the first-segment limitation sentence from `ai-gateway/docs/text-observability.md` and state that the export covers every segment. Check `docs/concepts/gateway-architecture.md` and the `docs/concepts/gateway-glossary.md` "Response segment" entry for drift; no edit is expected. Verify with `mise run lint-docs`.
- [ ] 4.4 Run `mise run test-ai-gateway` and confirm the Gateway observability tests pass, including `TestStructuredOutput_IndependentOperatorAndConsumerCapture` (the #438 regression) under `go test -race -count=20`.

## 5. Integration checks

- [ ] 5.1 Run `mise run fmt-check`, `mise run vet`, `mise run lint`, `mise run test` and `mise run test-race`, and confirm they pass.
- [ ] 5.2 Run `mise run validate-parity-baseline` and confirm that `git status test/conformance` is clean: no fixture or expectation changes.
- [ ] 5.3 Run `openspec validate agentobservability-multi-segment-streams --strict` and confirm it passes.
- [ ] 5.4 Confirm release intent: run `mise run release-preview` and check that only `middleware/agentobservability` gets a library release, at the next alpha. Note any Gateway plugin attribution caused by the `ai-gateway` test and doc edits.
  - PR title: `feat(agentobservability)!: record every response segment of one stream`.
  - The PR body carries a `BREAKING CHANGE:` footer describing the timing contract (design D5), links #582 and #583, and records the parity classification (design D8).
- [ ] 5.5 Open the follow-up issue for exact per-message stream usage, with the `programmatic-tool-calling` evidence and the options from design D2. Link it from the PR description.

## Workflow follow-up

- Archive the change in the same PR before merge (after the 4.2 rebase re-expression), and verify the synced specs.
