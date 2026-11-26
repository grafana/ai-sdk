## Why

Applications can persist named UI data and convert it into prompt content with `ConvertToModelMessages`, but the Agent UI helpers currently discard that data during prompt preparation. The registered upstream helpers let the caller opt into the same converter without changing the UI continuation.

## What Changes

- Let both Agent UI stream helpers accept an explicit text/file data converter, separately from emission options.
- Reuse UI-to-model conversion after validation and keep the original normalized UI history isolated from callback mutation.
- **BREAKING:** The helper variadics change to `...AgentUIStreamOption`. Individual existing emission options remain accepted; callers spreading a `[]UIMessageStreamOption` or relying on an exact function type must update it.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-tool-loop`: Add opt-in data conversion to both UI stream helpers.

## Impact

Root Go Agent UI API, focused prompt/assembly tests and schema-parsed frontend interoperability. No wire/schema, provider, or baseline changes.
