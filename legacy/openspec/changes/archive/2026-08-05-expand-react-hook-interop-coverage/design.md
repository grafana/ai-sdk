## Context

`test/integration/react-hooks.test.tsx` currently sends real HTTP requests from `@ai-sdk/react` hooks to deterministic handlers in `test/integration/testserver/`, but it asserts only successful final values. The registered runtime baseline is `ai@7.0.44` and `@ai-sdk/react@4.0.47` in `test/conformance/upstream.yaml`. Upstream source contracts were inspected at registered commit `c527d7b3b26287598d2c80e7bce8f16b21653363`; package manifests at that git object carry earlier version labels, so the npm pins remain the runtime authority and the source object is cited only as the registered behavioral reference.

This work crosses the Go scenario server, React probes, and parity metadata. It does not require provider calls or provider conformance fixtures because the behavior under test is the frontend hook state machine at the Go HTTP boundary.

## Goals / Non-Goals

**Goals:**

- Exercise the issue's chat, completion, and object lifecycle contracts through the pinned React hooks.
- Make intermediate and cancellation states reproducible without shared mutable test-server state.
- Prove approval response state and resumptions for both approved and denied decisions.
- Make `test/conformance/PARITY.md` accurately distinguish hook-level evidence from lower-level wire and reader evidence.

**Non-Goals:**

- Change public Go APIs, production stream wire formats, provider behavior, or package versions.
- Add synthetic inputs under recorded or upstream provider conformance fixture directories.
- Claim exhaustive hook parity from the added scenarios.
- Upgrade or reconcile the registered git object and npm package version labels.

## Decisions

### Use focused scenario routes backed by production stream writers

Add focused handlers under `test/integration/testserver/` for an HTTP failure, a UI-stream error, a controlled/abortable UI stream, an approval flow, a controlled/abortable text stream, and schema-invalid object JSON. Keep `simple-text` as the existing final-success assertion, use the controlled UI stream for lifecycle and stop assertions, and extend the existing `agent-tool` test for step-boundary snapshots where possible.

UI success/error scenarios will use the existing Go orchestration and UI response helpers, with mock `provider.LanguageModel` streams supplying deterministic parts. Completion and object scenarios will use the existing text response path. The HTTP failure route may be shared by chat and completion because non-2xx handling is independent of stream protocol; stateful stream behavior remains in protocol-specific routes.

This is preferred over a query-driven mega-handler because each route documents one failure or lifecycle contract. It is preferred over hand-authored provider events because provider fixture provenance is irrelevant here and production Go stream conversion should stay in the exercised path.

### Make phase boundaries explicit and cancellation context-aware

Controlled handlers will flush headers and an initial partial chunk before holding the response open at a request-scoped phase boundary. Any bounded wait or channel send will select on `r.Context().Done()`, and mock-model producer goroutines will exit when the request is canceled. Successful lifecycle scenarios will release/close after a bounded phase; stop scenarios will cancel while the partial output is visible. No package-global controller, execution counter, or cross-request gate will coordinate tests.

React probes will record immutable status/message/loading/callback snapshots with `useEffect`. Assertions will wait for observable states rather than sleep in the test. Repeated focused integration runs will detect residual timing flakiness.

This is preferred over uncoordinated sleeps, which can skip intermediate states or leak blocked goroutines. A fully client-addressable control endpoint would add shared synchronization and test isolation complexity that is unnecessary for these bounded scenarios.

### Assert the hook contracts at their public surface

The React tests will assert:

- Successful `useChat` status history contains the ordered subsequence `submitted`, `streaming`, `ready`.
- Both a non-2xx response and a UI `error` chunk expose the expected `Error` and end in chat status `error`.
- Calling chat `stop()` after partial text retains that text, prevents later text from appearing, and returns status to `ready`.
- Immutable message history exposes the first step boundary before the next step and preserves the expected tool input/output and final text across the existing agent flow.
- Approval tests configure `sendAutomaticallyWhen: lastAssistantMessageIsCompleteWithApprovalResponses`, call `addToolApprovalResponse({ id, approved, reason })`, and observe `approval-requested` followed by `approval-responded`. The approved branch preserves `approved: true` and its reason before producing `output-available`; the denied branch preserves `approved: false` and its reason before producing `output-denied` without a successful tool output.
- A completion HTTP failure calls `onError` once with the expected `Error`, does not call `onFinish`, populates the hook error, and resets `isLoading` to false. Completion stop ignores the abort as an error, resets loading, and retains only the partial completion.
- A completed schema-invalid object calls `onFinish` once with `object: undefined` and `error` matching `Error`; this callback result, rather than only the rendered partial object, is the contract.

Capturing callback arguments and state histories is preferred over checking a final DOM string because final output alone cannot prove lifecycle ordering, reset behavior, or exact callback semantics.

### Treat approval as a stateless two-request flow

The approval handler will decode posted UI messages using the same request conversion path as `scenario_agent_tool.go`. The first request emits a local tool call requiring approval. After the hook records the approval response, automatic resubmission carries the updated message history; Go approval orchestration resolves it before the next model call. Approved and denied results will be derived from that request history, not from server-global flags.

This matches the registered upstream `addToolApprovalResponse` behavior and avoids cross-test contamination. Approved and denied cases may use the same route because the decision is explicit in each second request.

### Narrow parity classifications to the evidence actually present

Update the `useChat`, `useCompletion`, `useObject`, and broad chunk/state-transition rows in `test/conformance/PARITY.md` to `mixed`. Their confidence text will enumerate the newly automated hook scenarios while noting that lower-level `expected.jsonl`, SSE parser, and UI-message reader tests prove wire/chunk behavior rather than full React state machines.

No conformance snapshot regeneration is planned because emitted production wire behavior is not changing. `mise run validate-parity-baseline` and `mise run parity-check` remain required to validate metadata and the pinned consumers.

## Risks / Trade-offs

- **[Risk] Browser scheduling can make short-lived intermediate states flaky.** → Flush distinct server phases, record histories instead of sampling only current state, make waits context-aware, and run the focused hook file repeatedly.
- **[Risk] Aborted handlers or mock producers can leak goroutines or attempt writes after cancellation.** → Select every blocked wait/send on request context and return promptly when canceled.
- **[Risk] Approved and denied tests can pass while exercising only local React mutation.** → Enable automatic resubmission and assert the final Go-orchestrated output/denial result as well as `approval-responded`.
- **[Risk] Broad parity wording can still imply exhaustive coverage.** → Keep all four broad rows `mixed` and name the remaining lower-level or untested surface explicitly.
- **[Trade-off] The registered git source manifests and npm runtime pins have different version labels.** → Treat npm `7.0.44`/`4.0.47` as executable evidence and the registered commit as behavioral source evidence; do not turn this test change into a baseline upgrade.

## Migration Plan

The change is additive test infrastructure plus documentation. Land focused server routes and hook tests together, update the parity map after the assertions exist, then run repeated focused integration tests and the full parity checks. Rollback consists of reverting the new tests/routes and parity wording; there is no production rollout or data migration.

## Open Questions

None. Route names and small probe structures may follow adjacent conventions during implementation without changing the specified behavior.
