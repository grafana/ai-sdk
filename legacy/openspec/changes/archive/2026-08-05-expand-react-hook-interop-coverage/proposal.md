## Why

The pinned React integration suite proves successful stream consumption but does not exercise hook state transitions, failures, cancellation, approval responses, or final object validation. As a result, `test/conformance/PARITY.md` overstates broad frontend interop coverage as automated and could hide regressions at the actual `@ai-sdk/react` boundary.

## What Changes

- Add deterministic Go test-server scenarios and React-hook integration assertions for `useChat` status ordering, HTTP and stream failures, cancellation with retained partial output, step-boundary message state, and approved and denied tool approval flows.
- Add `useCompletion` integration assertions for error callbacks, loading reset, cancellation, and retained partial completion.
- Add a `useObject` integration assertion for a completed object that fails its schema, including the exact `onFinish` result.
- Reclassify broad frontend parity rows as `mixed` and describe which behavior is proven at hook level versus only by lower-level wire, reader, or conformance evidence.
- Keep the change limited to deterministic integration evidence and parity metadata unless a new test exposes a production defect.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `integration-testing`: Require deterministic controlled-stream scenarios and React-hook coverage for lifecycle transitions, failures, cancellation, approval responses, and final schema mismatch.
- `upstream-parity-governance`: Require frontend parity classifications to distinguish hook-level evidence from lower-level stream evidence and avoid broad `automated` claims when coverage remains partial.

## Impact

- Test server scenarios under `test/integration/testserver/`.
- React integration probes and assertions in `test/integration/react-hooks.test.tsx`.
- Frontend coverage classifications in `test/conformance/PARITY.md`.
- No planned public API, production wire-format, provider fixture, dependency, or baseline-version changes.
