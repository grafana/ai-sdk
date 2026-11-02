# Tasks

Each numbered group from 1 to 6 is one commit. Every commit must leave these passing:
- `mise run test-ai-gateway`;
- `mise run test-providerwire-v4`;
- `go test -race` for `ai-gateway/internal/...`, `ai-gateway/providerwire/v4/...` and `ai-gateway/cmd/grafana-ai-gateway/internal/...`, run from `ai-gateway` as `go test -race ./internal/... ./providerwire/v4/... ./cmd/grafana-ai-gateway/internal/...`.

No commit may edit an existing ProviderWire test file. `git diff --name-status origin/main -- 'ai-gateway/providerwire/v4/*_test.go'` must list only added files. Existing `internal/execution` fixtures may be made lifecycle-valid without changing what they assert (design, Goals). The branch is based on `main` after #583 (#393) and #581 (#482).

## 1. Tool history in the runtime

- [x] 1.1 Add `ToolHistory`, `RebuildToolHistory` and `ErrInvalidToolHistory` to `internal/execution`, moving the rules of `providerwire/v4/request.go:157-196` unchanged (design D1, D2). `ToolHistory` is not mutated after it is rebuilt, and each consumer copies it. Map the sentinel to invalid request with `ReasonNone` in `ClassifySelection`. Add a table test with one row per rule, plus the history cases:
  - deferred result;
  - orphan result, including a later call with the same ID;
  - conflicting duplicates: a re-emission after the result (`request.go:164-166`), a second re-emission, another name, either call not provider-executed, a mismatched or repeated result;
  - approval re-emission: once, and once followed by its result.

  Verify with `go test -race ./internal/execution/...`.
- [x] 1.2 Make `Runtime.Call` rebuild history before `applyPolicy`, and expose `Call.ToolHistory()`, which returns a copy. Add runtime tests:
  - bad history fails with zero selector and model calls;
  - bad history beside a protected header, gateway controls, and an ended context, fails as invalid request with no reason;
  - valid history is exposed unchanged, and changing the returned copy does not change the call's history.

  Verify with `go test -race ./internal/execution/...`.
- [x] 1.3 In ProviderWire, remove the pre-call history check from `ServeHTTP`, and pass `call.ToolHistory()` to the stream and unary paths. Keep `unresolvedProviderCalls` as a wrapper over `RebuildToolHistory` with today's signature, returning a non-nil empty map (design D8). Add a new test file asserting the byte-identical 400 document for bad history, unary and streaming, including with a protected header, gateway controls and a canceled context. Verify with `go test -race ./providerwire/v4/...` and `mise run test-providerwire-v4`.

## 2. Shared stream validator in `Stream.Next`

- [x] 2.1 Add `StreamValidator` to `internal/execution`, seeded from a copy of `ToolHistory`. Move the generic stream rules unchanged from `stream.go`, `stream_tools.go` and `reasoning.go` as they are after #583 (design D3, D7): start position, response segments and quiescence, the parts allowed after a `finish`, per-segment text and reasoning IDs, stream-wide tool and approval IDs, tool phases and history, tool-result JSON, approvals, finish preconditions, registered finish reason and unknown types. Error and raw parts change no segment, text, reasoning or tool state, but consume the start position. `SegmentFinished` reports the finished phase. The validator is nil-safe for every pointer field (design D1). Add a table test with one row per rule, including:
  - metadata and `finish` while text, reasoning or tool input is open or a preview is pending, in the stream or in history;
  - consecutive and late metadata opening segments; text and reasoning IDs reused in a later segment; a tool ID reused in a later segment; a result in a later segment;
  - after a `finish`: metadata, raw and error accepted; content, a second `finish` and `stream-start` rejected;
  - the history-seeded call, result, preview and approval rows, and tool input for a history ID whose result already arrived;
  - approvals with an empty ID and with a false dynamic or preliminary value;
  - `raw` then `stream-start`, and `error` then `stream-start`, both rejected;
  - two validators seeded from one history stay independent, and the history is unchanged.

  Verify with `go test -race ./internal/execution/...`.
- [x] 2.2 Add `ErrInvalidOutput`, `WaitInvalidOutput` and `WaitFinished` (design D3, D4, D5):
  - `Wait.Err` returns `ErrInvalidOutput` and nil respectively; update its doc (`runtime.go:208`) to say nil for `WaitPart` and `WaitFinished`;
  - `ClassifyProvider` maps `ErrInvalidOutput` to internal;
  - `Call.View(ErrInvalidOutput)` projects the success view;
  - `Stream.Next` validates each part with the call's history, closes the stream on a violation and returns `WaitInvalidOutput`. A provider close returns `WaitFinished` when the segment is finished and `WaitClosed` otherwise. Invalid output and finished are sticky without reading. Idle, deadline and cancellation are unchanged in every phase.

  In the same commit, so that a clean close stays clean: ProviderWire's `consumeStreamParts` returns on `WaitFinished` instead of checking `WaitClosed` with its phase (`stream.go:363`), and the test adapter returns success on `WaitFinished` instead of tracking `finished` (`internal/testadapter/adapter.go:63-82`).

  Make the existing `internal/execution` stream fixtures lifecycle-valid without changing what they assert: the bare `PartTextDelta` parts in `TestStreamNext_Outcomes` (`runtime_test.go:467`, `:517`) and `optionsModel.DoStream`'s finish without usage or reason (`translate_test.go:55`).

  Add runtime tests:
  - a violation cancels and starts one drain, without returning the part; a repeated wait returns invalid output without reading, and a repeated close starts no second drain;
  - close after a `finish` returns finished, also with raw and error parts after it, and a repeated wait returns finished again; close after a later metadata returns closed;
  - idle timeout and cancellation after a `finish` keep their outcomes;
  - a later segment and pass-through parts are delivered;
  - the single-candidate selected view;
  - classification.

  Verify with `go test -race ./internal/... ./providerwire/v4/... ./cmd/grafana-ai-gateway/internal/...` and `mise run test-providerwire-v4`.

## 3. ProviderWire stream delegation

- [x] 3.1 Give `newStreamState(history)` its own `StreamValidator`, seeded from a copy. Check each part exactly once (design D8):
  - `runStream` consults the validator only in its first-part real `stream-start` branch (`stream.go:331-346`);
  - `processStreamPart` consults it once for every other part, including a non-start first part, before dispatching to `processToolStreamPart`, `processApprovalRequest` and `processReasoningPart`, which keep only representation checks. Its unconditional `stream-start` refusal stays as plumbing.

  Set `state.phase` from the validator's `SegmentFinished` after each accepted part, as a projection for the white-box tests. Remove `quiescent`, `startSegment`, `allowedAfterFinish`, the ID, tool, history and approval maps, and the lifecycle-only conditions, keeping the checks listed in design D7, and never share the runtime's instance. Verify with `go test -race ./providerwire/v4/...` and `mise run test-providerwire-v4`.
- [x] 3.2 Add `providerwire/v4/lifecycle_agreement_test.go`. For one violating sequence per stream rule class, including the segment rules, `raw` then `stream-start` and `error` then `stream-start`, it asserts that the runtime's validator and `processStreamPart` on `newStreamState` reject at the same index, feeding a leading real `stream-start` to ProviderWire's validator the way `runStream` does, and that both agree on the finished phase after every accepted part. Through the runtime harness, it asserts identical frames for a lifecycle violation and for a content violation ProviderWire still owns, at the first part, at a later part and after a written `finish`. Verify with `go test -race ./providerwire/v4/...`.

## 4. Shared unary validation

- [x] 4.1 Add `ValidateGenerateResult` to `internal/execution`, moving the generic rules of `response.go:136-226` unchanged (design D6, D7). It is nil-safe, and a nil result is invalid output. Add a table test with one row per rule, a nil result, and history rows:
  - a result for a pending history call;
  - a re-emission with and without an earlier preview or final result;
  - an approval on a history call.

  Verify with `go test -race ./internal/execution/...`.
- [x] 4.2 Make `Call.Generate` validate after the context-outcome check, only for a non-nil result returned with a nil error, returning `nil, ErrInvalidOutput`. Make the existing unary fixtures lifecycle-valid without changing what they assert: `generateModel.DoGenerate`'s `&provider.GenerateResult{}` with an empty finish reason (`attempts_test.go:47`, and the same result at `attempts_test.go:303` and `runtime_test.go:198`, `:286`, `:331`) and `optionsModel.DoGenerate` (`translate_test.go:47`). Add runtime tests: a violation, an ended context winning over a violation, a history-seeded result, and the success view. Verify with `go test -race ./internal/execution/... ./cmd/grafana-ai-gateway/internal/service/...` (`translation_test.go:381` calls `call.Generate` over real provider conversion).
- [x] 4.3 Make `mapUnarySuccess` call `ValidateGenerateResult` with its context's history, keeping only its representation and content-validity checks, and remove the lifecycle-only `unaryToolState` conditions. Extend the agreement test with one result per unary rule class, rejected by both. Add a result that fails a representation check and a lifecycle check, and assert that it writes the same 500 document and metadata as a result that fails only one. Verify with `go test -race ./providerwire/v4/...` and `mise run test-providerwire-v4`.

## 5. Test adapter evidence

- [x] 5.1 Make `testadapter.Run` report `WaitInvalidOutput` and unary `ErrInvalidOutput` in `Outcome`, as an invalid-output flag and the number of accepted parts (design D9). Verify with `go test -race ./internal/...`.
- [x] 5.2 Add a prompt field to the scenario in `internal/execution/adapter_test.go` (`:223-239`; `request` at `:325-346` always sends `"prompt": []`), encoded both as the wire prompt and as `provider.Message` values. Make `assertProviderWire` accept a `finish` frame before a terminal error and check that the terminal error is the last frame. Extend `TestTestAdapter_MatchesProviderWire` with rows for:
  - a first-part and a later-part violation for each stream rule class, including the segment rules;
  - a multi-segment stream that ends cleanly, and one that closes after a later metadata;
  - content after a `finish`;
  - a provider that stalls after a `finish` until the request is canceled, reported as cancellation by both;
  - a history-seeded stream and unary result;
  - inconsistent history (invalid request, zero model calls), unary and streaming;
  - unary invalid output.

  The rows leave out the content-validity classes of design D7 on purpose. For invalid output, the test adapter's accepted parts must equal the provider parts ProviderWire processed before its internal error, counted from its frames with the rules of design D9; rows with raw parts derive the raw count from the scenario input, because the harness does not request raw chunks. Verify with `go test -race ./internal/...`.

## 6. Documentation

- [x] 6.1 Update the `internal/execution` package godoc: the runtime owns tool-history reconstruction, stream and unary lifecycle validation, invalid output and the clean-close decision, and adapters own representation checks. Make the adapter list (`internal/execution/doc.go:20-21`) request mapping, representation, wire encoding, framing, error documents, response commitment and their own protocol state machine, and state that provider output lifecycle rules belong to the runtime. Update the `providerwire/v4/doc.go` sentences that describe its segment rules and say it owns "the stream state machine", so they say the runtime enforces the segment lifecycle and ProviderWire owns representation and its stream encoding. Verify with `go doc ./internal/execution` and `go vet ./internal/execution/... ./providerwire/v4/...`.
- [x] 6.2 In `docs/concepts/gateway-architecture.md`, change the stream flowchart and the paragraph on the ProviderWire stream encoder so the runtime validates the lifecycle, including segments, and decides the clean close, and ProviderWire checks representation. In `docs/concepts/gateway-glossary.md`:
  - extend the execution-runtime row (`:18`), keeping "its own protocol state machine" in its adapter list;
  - point the "Response segment" row at the runtime validator;
  - add an "Output lifecycle validation; invalid output" row linking the validator source.

  Verify with `mise run lint-docs`.

## 7. Integration checks (checks only, not a commit)

- [x] 7.1 Run `mise run check`, `mise run lint`, `mise run lint-docs`, `mise run parity-check`, `mise run validate-parity-baseline`, `mise run test-integration`, `mise run test-ai-gateway`, `mise run test-ai-gateway-command`, `mise run test-providerwire-v4` and `mise run test-race`. `mise run test-race` uses `-short` (`mise.toml:161-165`), so also run the non-short Gateway race command from the header: `(cd ai-gateway && go test -race ./internal/... ./providerwire/v4/... ./cmd/grafana-ai-gateway/internal/...)`.
- [x] 7.2 Confirm the following:
  - `git diff --name-status origin/main -- 'ai-gateway/providerwire/v4/*_test.go'` lists only added files;
  - `git diff --stat origin/main -- test/conformance` is empty, so `upstream.yaml`, fixtures and PARITY.md are unchanged (design D10);
  - `rg` finds `historicalToolCall`, `toolStreamPhase`, `toolStreamState`, `toolInputOpen`, `toolResultPreliminary`, `segmentTextIDs`, `segmentReasoningIDs`, `activeID`, `reasoningIDs`, `approvalIDs`, `quiescent`, `allowedAfterFinish` and `unaryToolState` nowhere in non-test `ai-gateway/providerwire/v4` code.

## Workflow follow-up

- Open a draft PR titled `refactor(ai-gateway): share provider output lifecycle validation`, based on `main`, linking #483, #394 and #583.
- Archive the change in the same PR before merge, and verify the synced `gateway-execution-runtime` spec. When syncing, update its Purpose line (`openspec/specs/gateway-execution-runtime/spec.md:4`) to add provider output lifecycle validation to the runtime (design D7).
