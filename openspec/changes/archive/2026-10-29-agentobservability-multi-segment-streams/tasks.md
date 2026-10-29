# Tasks

## 1. StreamRecorder multi-segment state (`middleware/agentobservability/map_stream.go`)

- [x] 1.1 Write the `map_stream_test.go` tests first and run `cd middleware/agentobservability && go test -run 'TestStreamRecorder_MultiSegment' ./...` against the current code. Use synthetic parts only; do not invent provider fixtures.
  - `TestStreamRecorder_MultiSegment_ReusedIDsAppendSeparateParts` (fail-first: the current code merges reused IDs):
    - two segments each emit text and reasoning with ID `"0"` and end in `PartFinish`;
    - output is the segment-1 parts followed by separate segment-2 parts, in observed order;
    - a tool call in segment 1 with its result in segment 2 still pairs.
  - Multi-finish usage is covered by task 6.3 (`TestStreamRecorder_MultiSegment/LastFinishUsageAndIdentity`).
  - `TestStreamRecorder_MultiSegment/LastFinishUsageAndIdentity` (its identity and finish-reason assertions pass before and after the change):
    - the last finish reason maps to `StopReason`;
    - the last non-empty response ID and model win.
- [x] 1.2 Implement design D4 and verify that 1.1 and the existing `TestStreamRecorder_*` tests pass (`go test ./...` in the module).
  - `PartFinish` replaces the `texts`/`reasonings` maps.
  - `streamOutputRef` holds the text or reasoning accumulator pointer, and `buildOutput` reads through it instead of `r.texts[ref.id]`/`r.reasonings[ref.id]`.
  - Tool accumulators stay keyed for the whole stream.
  - Update the `StreamRecorder` godoc for segment handling.
- [x] 1.3 Add multi-finish usage assertions to `TestStreamRecorder_MultiSegment/LastFinishUsageAndIdentity`. Task 6.3 sets its final expectation to the last finish's usage. Verify with `go test -run TestStreamRecorder_MultiSegment ./...`.

## 2. Recording finalization (`middleware/agentobservability/recording.go`)

- [x] 2.1 Write `recording_test.go` tests that use `testkit.NewEnv` and wait for `OnRecordComplete` before `Shutdown`. Run them against the current code: fail-first tests must fail, and regression pins must pass before and after the change.
  - `TestRecordingMiddleware_StreamMultiSegment_OneGeneration` (fail-first): two segments, then close. Expect:
    - exactly one export;
    - the second finish's usage (task 6.3) and stop reason;
    - output from both segments;
    - `completed_at` not before the observation of the second finish;
    - first-token time from segment 1, read from the env metrics or spans (time-to-first-token), with a measurable gap between segments.
  - `TestRecordingMiddleware_StreamStallAfterFinish_RecordsSnapshot` (regression pin: the current code ends at the finish): one segment, then the upstream blocks until the context is canceled. Expect:
    - no `call_error`;
    - the first segment's usage and finish reason;
    - `completed_at` at the finish, before the cancel. Use a gap the test can measure.
  - `TestRecordingMiddleware_StreamErrorAfterFinish_RecordsError` (fail-first): finish, then `PartError`, then cancel before close. Expect `call_error` to carry the stream error, not the cancellation.
  - `TestRecordingMiddleware_StreamErrorBeforeFinishThenCancel_RecordsError` (regression pin: the current code applies `CallError()` at the first finish): `PartError`, then `PartFinish`, then cancel before close. Expect `call_error` to be the stream error, not a cancellation, with the snapshot's usage and finish reason.
  - `TestRecordingMiddleware_StreamErrorAfterOnlyFinishThenClose_RecordsError` (fail-first: the current code stops observing at the finish): one `PartFinish`, then `PartError`, then a normal close. Expect `call_error` to be the stream error.
  - `TestRecordingMiddleware_StreamCloseAfterContextEnded_RecordsSnapshot` (regression pin, outcome only): finish, then the context ends, then upstream closes (`canceled = ctx.Err() != nil`). Expect the snapshot outcome with no `call_error`. The test cannot force this branch, because `select` picks randomly between the close and `ctx.Done`; both give the same outcome.
  - `TestRecordingMiddleware_StreamConsumerStopsReadingWithoutCancel`:
    - first case (regression pin): the consumer stops after the finish without canceling, and upstream then closes. The recording completes, and the `OnRecordComplete` signal is the wait condition.
    - second case (fail-first): upstream stays open. Assert that no generation is exported before the cancel (the current code exports at the finish), and that it is exported after the cancel.
  - `End` panic on close and snapshot paths (regression pin: the current code already recovers in `finalize`): extend `TestRecordingMiddleware_FinalizePanic` in `stream_panic_test.go`, or add a sibling, with a multi-finish stream on both the close path and the finish-then-cancel snapshot path. The panic comes from `panicTracer` span `End`, reached only through `finalize`/`endRecording`, so this does not cover snapshot capture (its guard is defensive and untested, design D1). The stream passes through unchanged, and the goroutine does not crash.
- [x] 2.2 Adapt `TestRecordingMiddleware_StreamFinishIsRecordedBeforeDownstreamCancels`, renaming it to `TestRecordingMiddleware_StreamStopAtFinishThenCancelRecordsSuccess`, at drain timeouts 0 and 10ms:
  - the consumer breaks at finish and cancels;
  - the test waits for `OnRecordComplete` before `Shutdown`;
  - it asserts one generation with no `call_error`.

  Keep `TestRecordingMiddleware_StreamConsumerAbandons_RecordsCancellation`, `TestRecordingMiddleware_StreamCancellationFinalizesIdleProvider` and `TestRecordingMiddleware_StreamCancellationTakesPrecedenceOverPartError` unchanged. These cover cancellation before any finish, and they must still pass.
- [x] 2.3 Implement design D1 in `runStreamTee` and verify with `cd middleware/agentobservability && go test -race ./...`. All 2.1/2.2 tests, `TestRecordingMiddleware_FinalizePanic`, `TestRecordingMiddleware_StreamSuccess_RecordsAccumulated`, the usage-export tests and the conformance tests must pass unchanged.
  - Remove the `finished` gate.
  - At each `PartFinish`, capture a stream-derived snapshot (D1) under `streamrecover.Recover`: the built output, usage, finish reason, response identity, `CallError()`, `FirstChunkAt()` and the finish time. Do not call `Generation()` per finish. Mark the snapshot invalid on panic.
  - Track a `PartError` with non-nil `APICallError` seen since the last finish (the same condition that updates `callError`).
  - In `finalize`, compose prompt-derived fields once, apply the D1 call-error rule and table (falling back to current state with `streamRec.CallError()` for an invalid snapshot), filter once, and keep `End` before `close(tee)`.
  - Rewrite the deferred comment and the `RecordingMiddleware` godoc to describe the new timing contract and when to wait for `OnRecordComplete`.
- [x] 2.4 Update `docs/middleware/agent-observability.md` "Record generation relationships". Verify with `mise run lint-docs`. Describe:
  - one generation for streams with several finishes;
  - last-finish usage (task 6.4 rewrites the usage sentence);
  - the last finish reason;
  - draining to EOF or waiting for `OnRecordComplete` before closing the client.

## 3. Dependency check

- [x] 3.1 Re-run `go list -m -versions github.com/grafana/agento11y/go github.com/grafana/agento11y/go-providers/anthropic`.
  - If a release newer than v0.19.0 exists, bump both together in `middleware/agentobservability/go.mod` and the agento11y require in `ai-gateway/go.mod`, run `mise run tidy`, and run `mise run test-agent-observability-middleware` and `mise run test-ai-gateway`.
  - Otherwise record "no newer release" in the PR description.
  - Never hand-edit the `middleware/agentobservability` version or changelog.

## 4. Gateway spec, test and docs

- [x] 4.1 Update `TestModelObservationChain_SequentialSegments` in `ai-gateway/cmd/grafana-ai-gateway/internal/service/model_observability_test.go` (design D6). Today every case asserts generation input 7, output 2 and `stop_reason` "tool_calls". Verify with `cd ai-gateway && go test -race -run TestModelObservationChain_SequentialSegments ./cmd/grafana-ai-gateway/internal/service/...`.
  - `normal close` (fail-first): generation input 4 (`inputSecond`), output 9 (`outputSecond`) and `stop_reason` "stop", matching the log record's usage and finish reason (task 6.3).
  - `deadline stall after finish` and `idle stall after finish` (regression pins): generation input 7, output 2 and "tool_calls", and assert no `call_error` (`assert.Empty(t, generation["call_error"])`).
  - Add table fields for per-case usage (input, output), generation stop reason and call error instead of the shared assertions. Assert the same usage on the log record in every case.
  - Keep the wire, log and metric assertions of the three existing cases unchanged.
  - Add the case `provider error before finish, then deadline stall`. Its parts are a `PartError` carrying `provider.NewAPICallError` with status 503 and a message without private values, followed by `segments[:5]`. Use `stall: true`, `deadline: 100 * time.Millisecond` and `idle: 5 * time.Second`. Expect:
    - wire body: one `finish` and two `error` frames (the forwarded provider error and the terminal timeout); compare the finish position with the last error frame;
    - log event `aisdk.model.stream.error` with outcome "error" (the logger keeps the first `PartError`, `middleware/logger/stream.go:132-135`), and metric labels `error_type="context_deadline_exceeded"` and `status="canceled"`;
    - generation `call_error` "server_error" (the agento11y metadata-only classification of a 503), input 7, output 2 and "tool_calls".
- [x] 4.2 Replace the limitation sentence in `ai-gateway/docs/text-observability.md` "Export to Agent Observability" (:75-77, "When one stream carries several messages or responses, the export currently records only the first one's usage and finish reason; logs and metrics cover the whole stream."). State instead that one export covers every message or response in the stream, with the last finish reason and the last message's usage, the same usage logs and metrics report. Verify with `mise run lint-docs`.
- [x] 4.3 Check the Gateway architecture docs for first-segment export wording; no edit is expected. Re-run `rg -n -i "segment|first finish|agent observability" docs/concepts/gateway-architecture.md docs/concepts/gateway-glossary.md`. Today the only hits are runtime descriptions, not the export:
  - `docs/concepts/gateway-architecture.md:223-228`, the stream lifecycle paragraph ("Each `response-metadata` part starts a response segment and each `finish` ends one");
  - `docs/concepts/gateway-glossary.md:52`, the "Response segment" entry ("A `finish` ends a segment, not the stream").

  Edit them only if they describe the Agent Observability export.
- [x] 4.4 Run `mise run test-ai-gateway` and confirm the Gateway observability tests pass, including `TestStructuredOutput_IndependentOperatorAndConsumerCapture` (the #438 regression) under `go test -race -count=20`.

## 5. Integration checks

- [x] 5.1 Run `mise run fmt-check`, `mise run vet`, `mise run lint`, `mise run test` and `mise run test-race`, and confirm they pass.
- [x] 5.2 Run `mise run validate-parity-baseline` and confirm that `git status test/conformance` is clean: no fixture or expectation changes.
- [x] 5.3 Run `openspec validate agentobservability-multi-segment-streams --strict` and confirm it passes.
- [ ] 5.4 Confirm release intent: run `mise run release-preview` and check that the root module, `middleware/logger`, `middleware/prometheus` and `middleware/agentobservability` get library releases, at the next alpha. Note any Gateway plugin attribution caused by the `ai-gateway` test and doc edits.
  - PR title: `feat(middleware)!: record every response segment of one stream`.
  - The PR body carries a `BREAKING CHANGE:` footer describing the usage rule (design D2) and the timing contract (design D5), links #582, and records the parity classification (design D8).

## 6. Last-finish stream usage (`internal/streamusage`)

- [x] 6.1 Rewrite `internal/streamusage/usage_test.go` first and confirm the new cases fail against the current aggregator. Cases:
  - before any finish, the greatest value per counter (pin);
  - finish counters replace lower provisional ones, and counters the finish omits keep the segment's greatest value;
  - a finish without usage closes the segment with its greatest counters;
  - across several finishes the last segment wins, including Anthropic's carried output on response metadata (fixture-shaped 3369/725, then 4551/725 metadata and a 4551/197 finish, gives 4551/197);
  - usage after the last finish does not change the aggregate;
  - raw usage: latest non-empty before a finish, and the finish's raw payload when it reports one.
- [x] 6.2 Implement design D2 in `Aggregator` (segment state, last closed segment, finish-reported counters) and verify with `go test -race ./internal/streamusage/`.
- [x] 6.3 Update the consumer tests to the new rule. Each keeps its purpose:
  - the provisional-lower usage tests in `middleware/logger` (`TestMiddleware_StreamUsageTakesFinishCounters`), `middleware/prometheus` (`TestStreamUsageTakesFinishCounters`) and `middleware/agentobservability` (`TestStreamRecorder_UsageTakesFinishCounters`) expect the finish counters, with `NoCache` from the earlier part;
  - `TestStreamRecorder_MultiSegment/LastFinishUsageAndIdentity` and `TestRecordingMiddleware_StreamMultiSegment_OneGeneration` expect 4/9;
  - `TestRecordingMiddleware_RequestedStreamLifecycle` and the stream case of `TestRecordingMiddleware_InclusiveUsageExport` use a finish without usage, so their split-usage expectations stay;
  - `TestModelObservationChain_SequentialSegments` (task 4.1).
- [x] 6.4 Update the usage sentences in `docs/middleware/structured-logging.md`, `docs/middleware/prometheus.md` and `docs/middleware/agent-observability.md`, and the export paragraph in `ai-gateway/docs/text-observability.md`. Verify with `mise run lint-docs`.
- [x] 6.5 Re-run task 5.1's checks and `mise run test-ai-gateway`.

## Workflow follow-up

- Archive the change in the same PR before merge, and verify the synced specs: `rg -n "first finish|first segment|#582" openspec/specs/gateway-text-observability/spec.md` returns only the new rules ("first-output timing from the first segment"), and `openspec validate --all --strict` passes.
