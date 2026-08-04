## 1. Deterministic Go Scenarios

- [ ] 1.1 Add focused non-success HTTP and Go UI-stream error scenarios under `test/integration/testserver/`, using the existing response/orchestration paths rather than provider conformance fixtures.
- [ ] 1.2 Add controlled UI and text stream scenarios that flush deterministic partial output, separate later output with a bounded request-scoped phase, and stop all blocked work on request cancellation.
- [ ] 1.3 Add a schema-invalid object text scenario that completes with valid JSON and an approval scenario that decodes posted UI history and resolves approved or denied responses without shared server state.

## 2. React Hook State-Machine Coverage

- [ ] 2.1 Extend `test/integration/react-hooks.test.tsx` probes with immutable histories for status, messages, loading, errors, and callback arguments while preserving the existing success assertions.
- [ ] 2.2 Add `useChat` assertions for ordered `submitted -> streaming -> ready`, non-success HTTP and UI-stream errors ending in `error`, and `stop()` returning to `ready` with only the partial assistant text retained.
- [ ] 2.3 Extend the multi-step tool assertion to prove first-step message state before the next step, then add approved and denied `addToolApprovalResponse` flows that assert `approval-requested -> approval-responded`, response data, automatic resubmission, and final output or `output-denied` state.
- [ ] 2.4 Add `useCompletion` assertions that an HTTP error calls `onError` once, does not call `onFinish`, exposes the error, and clears loading, and that `stop()` clears loading without an error while retaining only the partial completion.
- [ ] 2.5 Add a `useObject` schema-mismatch assertion that captures exactly one final `onFinish` result with `object: undefined` and an `Error`.

## 3. Parity Evidence Classification

- [ ] 3.1 Update the `useChat`, `useCompletion`, `useObject`, and chunk ordering/state-transition rows in `test/conformance/PARITY.md` to `mixed`, enumerating the new hook-level assertions and the remaining partial surface.
- [ ] 3.2 Keep lower-level conformance snapshots, SSE/parser evidence, and UI-message reader evidence distinct from React hook evidence; confirm the diff does not alter provider fixture inputs, package pins, or production wire behavior unless a newly reproduced defect is separately evidenced and classified.

## 4. Validation

- [ ] 4.1 Format changed Go and TypeScript files and run `cd test/integration/testserver && go test ./...`.
- [ ] 4.2 Run the focused hook suite at least five consecutive times with `cd test && pnpm --filter @ai-sdk/test-integration exec vitest run react-hooks.test.tsx` to detect phase/cancellation flakiness.
- [ ] 4.3 Run the complete cross-language integration suite with `mise run test-integration`.
- [ ] 4.4 Run `mise run validate-parity-baseline` and `mise run parity-check`.
- [ ] 4.5 Run `openspec validate expand-react-hook-interop-coverage --type change --strict --no-interactive` and inspect the final diff for scope and evidence accuracy.
