# Design

## Context

- The lifecycle rules live in ProviderWire V4, interleaved with encoding. On main after #583 they are:
  - prompt history: `unresolvedProviderCalls` in `providerwire/v4/request.go:157-196`;
  - the stream state machine: `streamState`, `quiescent`, `startSegment` and `allowedAfterFinish` (`stream.go:261-315`), `processStreamPart` (`stream.go:387-534`), `processToolStreamPart` and `processApprovalRequest` (`stream_tools.go:69-172`), and `processReasoningPart` (`reasoning.go:61-90`);
  - the unary mapper: `mapUnarySuccess` in `response.go:102-240`.
- #583 (closing #393) replaced the single-response rules with response segments (archived change `2026-10-28-gateway-sequential-response-segments`, D1-D6):
  - each `response-metadata` starts a segment and each `finish` ends one; both require quiescence: no open text, reasoning or tool input, and no pending preview, in the stream or in history;
  - text and reasoning IDs are unique per segment, tool and approval IDs per stream;
  - after a `finish`, only `response-metadata`, raw or provider error parts may follow;
  - provider close is a clean end only when the last lifecycle part is a valid `finish`. Idle timeout, deadline and cancellation after a `finish` still end with their errors.
- `ServeHTTP` checks history after mapping and before `runtime.Call` (`handler.go:87-92`). `Runtime.Call` then applies host policy, checks the context and selects (`internal/execution/runtime.go:67-88`). Any history failure writes `invalidMappingFailure()`, which is `safeInvalidRequest` with no metadata.
- `Stream.Next` (`runtime.go:238-260`) returns one of five waits. The clean-close decision is made twice outside the runtime:
  - ProviderWire ends cleanly on `WaitClosed` when its phase is `segmentFinished` (`stream.go:363`);
  - the test adapter repeats that decision with its own `finished` flag (`internal/testadapter/adapter.go:63-82`).
- ProviderWire handles rejection the same way for every rule: `streamPartAdapterFailure` closes the stream and writes one internal terminal error with the view's metadata (`stream.go:376-385`). A post-finish violation therefore writes the `finish` frame, then one `internal_error`.
- The attempt view is sealed after the first wait as `call.View(wait.Err())` for streams, and as `call.View(nil)` before unary mapping (`handler.go:108`). For a single candidate, `View` synthesizes a failed attempt when the error is non-nil (`attempts.go:91-100`).
- Several required-unchanged tests are white-box. They call ProviderWire internals directly and assert lifecycle rules:
  - `processStreamPart` on `newStreamState(nil)`: `TestReasoningFrameEncodingAndLifecycle` (`reasoning_test.go:152`), and the finish cases in `reasoning_test.go`, `metadata_test.go` and `generated_file_test.go`, which since #583 also read `state.phase == segmentFinished`;
  - `mapUnarySuccess`: `TestUnarySuccessValidation` (`response_test.go:64`) and others;
  - `unresolvedProviderCalls`: `TestRuntimeProviderToolHistory` and `TestUnresolvedProviderCalls_RejectsAmbiguousHistory` (`function_tools_test.go:170`, `:229`).

## Goals / Non-Goals

**Goals:**
- One implementation of each lifecycle rule in `internal/execution`. The runtime enforces it for every adapter, and an adapter cannot bypass it.
- The runtime, not each adapter, decides whether a provider close is a clean end.
- Byte-identical ProviderWire output, error precedence and attempt views. The existing ProviderWire test files are not edited.
  - "Unchanged" means the ProviderWire lifecycle test files the issue names, as they are on main after #583: no existing `providerwire/v4` `*_test.go` is edited, including the segment and segment-replay tests.
  - Existing `internal/execution` fixtures are runtime-test inputs and may be made lifecycle-valid without changing what they assert: the bare `PartTextDelta` in `TestStreamNext_Outcomes` (`runtime_test.go:467`, `:517`), `generateModel.DoGenerate`'s `&provider.GenerateResult{}` with an empty, unregistered finish reason (`attempts_test.go:47`, used by `TestCallGenerate` and others), and `optionsModel`'s empty result and its finish without usage or reason (`translate_test.go:47`, `:55`).

**Non-Goals:**
- Changing, tightening or relaxing any rule, including the segment rules of #583.
- Adapter-specific rules, size accounting, and representation rules of other protocols.
- Changing the precedence of context, idle or drain outcomes.

## Decisions

### D1. API

```text
execution.RebuildToolHistory(prompt) (ToolHistory, error)  // ErrInvalidToolHistory
call.ToolHistory() ToolHistory                             // a copy: pending provider-executed calls, ID -> tool name
execution.NewStreamValidator(history) *StreamValidator     // Check(part) error; SegmentFinished() bool
execution.ValidateGenerateResult(result, history) error    // ErrInvalidOutput
stream.Next() (part, Wait)                                 // + WaitInvalidOutput, WaitFinished (terminal success)
```

- The names follow the repo rule of sentinels and `fmt.Errorf` only, with no custom error types. The validators are pure and protocol-neutral, and contain no encoding.
- `SegmentFinished` reports that the last lifecycle part accepted was a valid `finish`, the `segmentFinished` phase of #583.
- The validators are nil-safe for every pointer field they read (`FinishReason`, `Usage`, `Dynamic`, `Preliminary`): a nil value is either accepted or a violation, as today, never a panic. `ValidateGenerateResult(nil, history)` returns `ErrInvalidOutput`, matching the white-box `mapUnarySuccess(nil)` case (`response_test.go:82`).
- `ToolHistory` keeps today's shape: pending provider-executed calls by ID, with tool name. It is not mutated after it is rebuilt. Each validator copies it into its own state when seeded, and `call.ToolHistory()` returns a copy, so no two consumers share state.
- `WaitFinished` is a terminal success. Adapters do not classify it, and its `Err()` is nil; the `Wait.Err` doc (`runtime.go:208`, "nil for WaitPart") names both.

### D2. History first in `Runtime.Call`

- `Call` rebuilds history before `applyPolicy`, the context check and selection. On failure it returns `ErrInvalidToolHistory`, which `ClassifySelection` maps to invalid request with `ReasonNone`. The `Call` keeps the history.
- ProviderWire drops its own pre-call check. `safeErrorFromResolution` maps that category to `safeInvalidRequest`, and `writeSafeError(w, value, nil)` writes it, which is the same document as `invalidMappingFailure()`.
- Precedence is unchanged:
  - adapter mapping failures happen before history;
  - history comes before a protected call header or call-level `gateway` controls;
  - history comes before a context that has already ended. A cancelled request with bad history still gets 400, not 499.
  - Tests pin both combinations, and zero selector and model calls.
- **Alternative: rebuild history after `applyPolicy`.** Rejected: a protected header would then win over bad history.
- **Alternative: let the adapter call the builder.** Rejected: adapters could bypass it.

### D3. Stream validation inside `Stream.Next`

- `Next` passes each received part to the stream's validator, which carries the #583 rules unchanged:
  - **Lifecycle parts** are every part except raw and provider error parts. Raw and error parts change no segment, text, reasoning or tool state, but they consume the start position, as today: a non-start first part takes the synthetic-start branch (`stream.go:347-355`), and a later `stream-start` is refused (`stream.go:530`).
  - **Segments.** `response-metadata` is accepted only while quiescent; it opens a new segment and clears the per-segment text and reasoning IDs. `finish` is accepted only while quiescent and with its own preconditions; it sets the segment-finished phase. In that phase only `response-metadata`, raw and error parts are accepted; any other part, including a second `finish`, is invalid output.
  - **Identifier scope.** Text and reasoning IDs are unique per segment. Tool, history and approval IDs are unique per stream, so a provider-executed call can receive its result in a later segment.
- Accepted parts commit state and are returned as `WaitPart`. A violation closes the stream (cancel, plus the one drain) and returns `WaitInvalidOutput`; the part is not returned. `WaitInvalidOutput` is sticky: a later `Next` returns it again without reading, so it never competes with the drain that `Close` starts (`runtime.go:264-270`).
- **Check and commit happen in one step.** State advances before the adapter represents the part. The extra state is never observable, a requirement of the spec "Adapter representation checks":
  - an adapter that refuses a part as invalid ends the stream and never waits again;
  - an adapter may omit a part it does not represent, such as ProviderWire's raw drops (`stream.go:516-526`), only when the part carries no lifecycle state beyond the start position, which the runtime has already consumed either way.
  - **Alternative: separate check and commit-after-write calls.** Rejected: it leaves the runtime unable to enforce the rule without trusting the adapter.

### D4. The runtime decides how a stream ends

- A provider close returns `WaitFinished` when the validator's segment is finished, and `WaitClosed` otherwise. Both cancel the model context first, as every non-part outcome does. `WaitFinished` is a terminal success with a nil error, and sticky like `WaitInvalidOutput`: once the model context is canceled, a fresh read could race the closed channel against the canceled context, so a later `Next` returns the recorded outcome without reading. `WaitClosed` keeps today's behavior.
- Idle timeout, deadline and cancellation keep their outcomes in every phase, including after a `finish`. This is #583's D5: a stall between segments must never look like a complete response.
- ProviderWire ends cleanly on `WaitFinished` and writes the terminal error for `WaitClosed` as today. The test adapter reports success on `WaitFinished` and drops its `finished` flag.
- **Alternative: keep the decision in each adapter.** Rejected: it is a lifecycle rule, and the test adapter already had to copy it.
- **Alternative: reuse `WaitClosed` and expose the phase.** Rejected: every adapter would again need to know that a close after a `finish` is clean.

### D5. The invalid-output outcome

- `ErrInvalidOutput` is the error for `WaitInvalidOutput.Err()` and for unary violations. `ClassifyProvider` maps it to internal explicitly, which is what it already maps unknown errors to. The MODIFIED "Closed failure classification" requirement lists it, with inconsistent history under selection classification.
- `Call.View(ErrInvalidOutput)` projects the success view: the provider call succeeded and the Gateway refused its output. This keeps ProviderWire's metadata unchanged with no special case in the adapter:
  - a first-part violation used to seal the view with `View(nil)`;
  - unary used to call `View(nil)` before mapping.
- **Alternative: each adapter passes nil for invalid output.** Rejected: every adapter would need to know the rule.
- ProviderWire output stays identical:
  - a first-wait `WaitInvalidOutput` takes the existing non-part branch of `runStream`, so an empty `stream-start` is written, then the internal terminal error;
  - a later one, including one after a written `finish`, takes `writeStreamTerminalForWait`, which writes the same internal error and metadata as `handleStreamPartResult`.

### D6. Unary validation in `Call.Generate`

- After the existing context-outcome check, `Generate` runs `ValidateGenerateResult` with the call's history, only for a non-nil result returned with a nil error. A violation returns `nil, ErrInvalidOutput`, and a context that has ended still wins. Provider errors and the existing `nil, nil` → `errInternal` path (`runtime.go:129-139`) are unchanged. #583 did not change unary rules.
- ProviderWire's provider-error branch writes `safeInternal` with `call.View(err).Metadata()`, which D5 makes equal to today's `View(nil)` metadata.
- **Reject order.** Today metadata, warnings and identity fail before lifecycle checks. The runtime now checks lifecycle first. Every failure writes the same 500 document with the same metadata, so the order cannot be observed. A test pins a result that fails both kinds of check.

### D7. Which rules move

- **Scope reading.** The issue says ProviderWire keeps "only" UTF-8, metadata shape and the safe-integer bound. This change moves the lifecycle rules the issue enumerates, in their #583 form, and nothing else. The issue's "parts after a finish are suppressed" predates #583; the moved rule is "only metadata, raw or error after a finish".
- **Moved (generic lifecycle):** every rule listed in the spec delta, including tool-result JSON validity (non-empty, valid, not `null`) and the clean-close decision. Every adapter needs a result value it can parse.
- **Kept in ProviderWire (non-lifecycle content validity), unchanged:**
  - source validity, including a nil source (`stream.go:396-405`, `sources.go:48-51`);
  - the custom kind rule (`response.go:251-253`);
  - file data `Validate()` (`reasoning.go:57-59`);
  - the non-negative token count, in the same check as the safe-integer bound (`response.go:276-283`);
  - response identity and timestamp range (`response.go:126`);
  - the recovered-panic error-part refusal (`stream.go:474-476`).

  Consequence: the test adapter, and any second adapter, would forward a recovered panic as a provider error part today. The second Open Question asks whether these move to the runtime in a follow-up. Task 5.2's parity rows leave these classes out on purpose.
- **Kept in ProviderWire (representation):**
  - UTF-8 on every string and JSON value;
  - metadata, raw-usage and citation shape;
  - token counts at most 2^53-1;
  - the raw-part projection and the `includeRaw` drop;
  - warnings mapping;
  - the execution overview on every `finish` (#583 D7);
  - encoding and writer failures.
- **Kept in ProviderWire (adapter plumbing):** the synthetic `stream-start`; the validator only enforces the position.
- **Ownership wording.** The MODIFIED "Shared adapter runtime" requirement gives adapters representation, encoding, framing, commitment, error documents and their own protocol state machine, and gives the runtime provider output lifecycle validation. #394 keeps the protocol state machine with each adapter: it is the adapter's own protocol, such as Chat's chunk sequencing, not the provider output lifecycle. The main spec's Purpose line (`openspec/specs/gateway-execution-runtime/spec.md:4`) is updated in the same words when the change is archived and synced.

### D8. ProviderWire delegates to the same validators

- ProviderWire holds no copy of any rule, but its internals still evaluate them through `execution`, so the white-box tests stay unchanged:
  - `newStreamState(history)` owns its own `StreamValidator`, seeded from a copy of the history, and `processStreamPart` consults it before its representation checks;
  - `mapUnarySuccess` calls `ValidateGenerateResult` with its context's history;
  - `unresolvedProviderCalls` stays as a thin wrapper over `RebuildToolHistory`, with today's signature and its non-nil empty map.
- **`phase` stays as a projection.** The white-box tests read `state.phase == segmentFinished` after a `finish` (`reasoning_test.go`, `metadata_test.go`, `generated_file_test.go`). `processStreamPart` sets `phase` from its validator's `SegmentFinished` after each accepted part. Nothing in ProviderWire decides on it: the clean close comes from `WaitFinished` (D4). `quiescent`, `startSegment`, `allowedAfterFinish` and the ID and tool maps are removed.
- ProviderWire's instance sees exactly the provider parts the runtime saw, never the synthetic start. Each part is checked exactly once:
  - `runStream` consults the validator only in its first-part real `stream-start` branch (`stream.go:331-346`);
  - every other part, including a non-start first part, is checked once at the top of `processStreamPart`, before it dispatches to `processToolStreamPart`, `processApprovalRequest` and `processReasoningPart`, which keep only representation checks. Its unconditional `stream-start` refusal stays as plumbing.
- **Trade-off: in production, each rule runs twice.**
  - It costs about one extra map operation per part, and the second check always agrees: same code, same inputs, deterministic.
  - The two state instances must never be shared, because one instance would commit each part twice and reject valid streams.
  - The issue's "checks once" is read as one implementation.
- **Alternative: B (rejected).** Runtime-only enforcement, after which ProviderWire's redundant call and its `phase` projection are dropped. It removes the redundancy but edits every ProviderWire white-box test that asserts a lifecycle rejection or the phase through `processStreamPart`, `newStreamState` or `mapUnarySuccess`, tests the acceptance requires to stay unchanged. See Open Questions.
- **Alternative: an adapter-called validator only.** Rejected: adapters could bypass it.

### D9. Evidence

- **`execution` unit tests:**
  - one table row per rule in each validator, including every segment rule: quiescence for metadata and `finish`, per-segment ID reuse, stream-wide tool IDs across segments, the parts allowed after a `finish`, and a second `finish`;
  - the history cases: deferred result, orphan result, conflicting duplicates and approval re-emission;
  - two validators seeded from one history stay independent, and the history is unchanged;
  - runtime tests for drain on invalid output, sticky invalid output, `WaitFinished` on close after a `finish`, `WaitClosed` on close before one, idle, deadline and cancellation after a `finish`, the view, and precedence.
- **ProviderWire suites, unchanged:** including `stream_segments_test.go` and `stream_segments_replay_test.go`, which replays the seven upstream fixtures of #583 part by part.
- **Agreement test (new file in `providerwire/v4`):** for one sequence per rule class, the runtime's validator and ProviderWire's `processStreamPart` or `mapUnarySuccess` reject at the same index. Through the runtime harness, a lifecycle rejection writes the same frames as a content rejection ProviderWire still owns at the same part.
- **Test adapter:**
  - it maps `WaitFinished` to success with a zero category and drops its `finished` flag, because `ClassifyProvider(nil)` is internal (`classify.go:85-87`);
  - its `Outcome` reports invalid output and the number of accepted parts;
  - `TestTestAdapter_MatchesProviderWire` (`internal/execution/adapter_test.go:375`) gains rows per rule class, with segment, history, unary and after-finish rows. It leaves out the content-validity classes of D7 on purpose.
  - `assertProviderWire` accepts a `finish` frame before a terminal error, which #583 made valid, and checks that the terminal error is the last frame.
- **Parity count.** The test adapter's count is the parts `Next` delivered as `WaitPart` before `WaitInvalidOutput`. ProviderWire's count is the provider parts it processed before its internal terminal error, read from its frames: one frame per processed part, plus the leading `stream-start` frame, where
  - a real first `stream-start` is both a processed part and the leading frame, so it adds no extra frame;
  - a raw part with `includeRaw` off is processed and writes no frame, so the harness, which does not request raw chunks, takes the raw count from the scenario input;
  - an error part and a `finish` each write one frame.
- **Harness prompt.** The scenario struct has no prompt; the request always sends `"prompt": []` (`adapter_test.go:223-239`, `:325-346`). A prompt field, encoded both as the wire prompt and as `provider.Message` values for the test adapter, carries the history and inconsistent-history rows.

### D10. No upstream parity change

- **Classification:** Gateway-internal refactor, a parity-preserving Go adaptation. There is no `@ai-sdk/*` counterpart at the registered baseline, and the LanguageModelV4 stream-part contract is unchanged.
- `test/conformance/upstream.yaml`, its `knownGaps`, and the fixtures and expectations stay unchanged.
- The PARITY.md "AI Gateway" row (`PARITY.md:46`) keeps its status and evidence location, so it is not edited.

## Risks / Trade-offs

- [Double evaluation in ProviderWire] → The work is small and deterministic, and the agreement test proves the two evaluations agree. The open question below can remove it.
- [A rule drifts while moving] → The unchanged ProviderWire suites, including the #583 segment and replay tests, the per-rule tables, the agreement test and the test-adapter rows gate each commit.
- [The `phase` projection looks like lifecycle state in ProviderWire] → Nothing reads it except the white-box tests; the clean close comes from the runtime. Option B removes it.
- [View semantics for invalid output differ from other errors] → This is the existing ProviderWire behavior. The view and the classification tests pin it.
- [Validator state advances before representation] → It cannot be observed: the spec requires an adapter that refuses a part to end the stream, and lets it omit only parts with no lifecycle state beyond the start position.
- [Content-validity checks stay per adapter] → A second adapter forwards a recovered panic as a provider error part today. D7 and the second Open Question record it.

## Migration Plan

No operator or wire migration. Rollback is reverting the PR.

## Open Questions

- **For nara: is redundancy-free B preferred over A?** B edits every ProviderWire white-box test that asserts a lifecycle rejection or the segment phase through `processStreamPart`, `newStreamState` or `mapUnarySuccess`: about 50 call lines across 10 test files, including `reasoning_test.go:152`, `response_test.go:64`, `metadata_test`, `generated_file_test`, `native_response_test` and `raw_usage_test`, plus the `unresolvedProviderCalls` calls in `function_tools_test.go:170` and `:229`. They would go through the runtime or the `execution` validators, and ProviderWire would drop its delegated call and its `phase` projection. This can be decided after this change: the specs and other tasks stay the same, and it would be a follow-up that removes code.
- **For nara: should the recovered-panic error-part refusal and the content-validity checks (D7) move to the runtime in a follow-up?** Today a second adapter forwards a recovered panic as a provider error part, the weaker guarantee #394 criticises in #248. Moving them is a rule-ownership change outside this refactor's scope reading.
