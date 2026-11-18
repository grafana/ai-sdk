## Why

Repeated approval responses currently execute a local tool more than once or both execute and deny the same call. Issue #601 records the difference from registered `ai@7.0.153`.

## What Changes

- Collect only the first eligible approval response per tool call, after existing validation and result handling.
- Verify collection, real mock-tool execution, emitted denial events, and the next provider prompt across StreamText, GenerateText and ToolLoopAgent.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tool-approval-orchestration`: repeated responses resolve each call once.

## Impact

The shared core collector changes; no public API, provider adapter, or wire shape changes. Existing-result handling and the last-tool-message boundary stay intact. Execution ordering and concurrency belong to #444 and are non-goals.
