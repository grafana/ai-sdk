## Why

Callers can find the final generation step by indexing the steps, but currently must check for empty history themselves. A dedicated accessor makes per-step data available without confusing it with aggregate result fields or exposing a mutable internal step.

## What Changes

- Add `FinalStep() (StepResult, bool)` on streamed and collected generation results, including Agent results.
- Report absence when no step was recorded. Preserve completed step history when a stream later fails or is canceled.
- Return the step by value, with shared read-only nested data. Leave existing result fields, projections, lifecycle and wire behavior unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `stream-text-lifecycle`: Define the final recorded step accessor, including ownership, completion and absence.

## Impact

Additive root Go API and focused result tests. No provider, orchestration, UI or wire changes.
