## Why

Issue #259 remains valid at Go base `ec204b9164e9c7bdc74cb0cc416d55c0dedaffe1`: shared orchestration forwards tool choice but does not reject a completed response that omits a required call or the selected named call, allowing unrelated local tools to execute. The current registered upstream `ai@7.0.109` (`4e8c387622ee1bb0d55841664416d38754d5c9a3`) already rejects these responses while retaining completed model usage and metadata; this is an implementation bug, not a baseline upgrade.

## What Changes

- Validate completed responses against the effective per-step required/named tool choice, including `PrepareStep` overrides and later resets.
- Match the pinned existential predicate: required needs any parsed call; named needs at least one call with that name. A matching call plus unrelated calls is valid; invalid and provider-executed calls count toward presence. Auto/none behavior is unchanged.
- On violation, skip all new local execution and approval processing for that step, record the completed failed step, retain usage/metadata/content and raw finish, report an existing-style Go stream/result error, and finish with unified reason `error` without retry or continuation.
- Apply through `StreamText`, `GenerateText`, `ToolLoopAgent.Stream`, and `ToolLoopAgent.Generate`; preserve generate's `nil, error` contract with retained data in completion callbacks.
- Add conformance-first provider-independent UI regressions, focused entry-point/approval/lifecycle tests, and pinned schema-parsed frontend integration proof. Correct request-only tests that currently treat unsatisfied required/named responses as successes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `functional-options`: Define completed-response satisfaction using the existing effective step choice, independent of provider request mapping.
- `stream-text-lifecycle`: Finalize completed semantic failures with retained data and terminal error, and make choice violations an exception to approval processing on other disallowed finishes.
- `tool-approval-orchestration`: Prevent new local approval effects for a violating completed response while preserving provider-originated events and prior-message approval resolution.

## Impact

Primary implementation seam: `streamtext.go` choice resolution, `processStep`, completed-step recording/finalization, and `executeTools` gating. Generate and Agent delegate to that engine; prove their actual entry points rather than provider `DoGenerate`. Expected tests touch root tool-choice matrices and lifecycle/approval tests, `test/conformance/ui/`, the UI conformance generator/replay, and `test/integration/testserver/` plus Vitest scenarios.

No new public API or custom error type is approved. No provider mapping changes, package/pin upgrades, provider recordings, repair/refinement, caller-routing changes, or changes to approval execution from prior messages are included. Already streamed provider effects cannot be undone. Skipping new local approvals uses Go's existing post-stream batching safety boundary, not a claim of exact upstream streaming approval timing. `PARITY.md` changes during implementation only if durable evidence/support boundaries change; issue #259 retains work ownership. This change contains planning artifacts only; runtime implementation is deferred.
