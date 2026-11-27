# Proposal

## Why

Callers preparing a continuation currently have to filter model history themselves. Filtering only text reasoning misses reasoning files, and removing tool parts independently can leave a result or approval with no originating call.

## What Changes

- Add an explicit root SDK helper to prune reasoning, selected tool history, and zero-part model messages.
- Protect calls and approvals needed by the retained suffix according to the registered upstream rules.
- Leave generation, stream orchestration, and provider conversion unchanged when the helper is not called.

## Capabilities

### New Capabilities

- `model-message-pruning`: Caller-controlled pruning of model history before continuation.

### Modified Capabilities

None.

## Impact

Additive root SDK API and focused tests. No dependencies, provider types, fixtures, or wire formats change.
