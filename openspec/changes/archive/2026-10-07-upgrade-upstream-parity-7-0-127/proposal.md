## Why

The mature coherent upstream target advances the registered comparison contract. Its Anthropic adapter preserves server-side fallback boundaries, while Go currently drops them and fails the regenerated conformance fixture.

## What Changes

- Update the fixed package reference, consumer pins, lockfile and generated expectations together.
- Preserve Anthropic fallback custom content through unary output, streaming and assistant continuation without merging reasoning across model hops.
- Align persisted partial tool-input resume, raw-input lifecycle and Agent validation with the target frontend.
- Review supported surfaces and register remaining actionable differences independently; do not implement the parity backlog.

## Capabilities

### New Capabilities

- `anthropic-fallback-content`: Preserve server-side model-hop markers and validate their continuation metadata.

### Modified Capabilities

- `ui-message-stream-reader`: Persist/resume partial raw input and align error/input field clearing.
- `ui-tool-state-persistence`: Validate streaming raw input as a string without discarding it.
- `ui-message-conversion`: Drop unresolved approvals superseded by a later user message.

## Impact

Anthropic response/stream/prompt conversion, focused provider and frontend tests, conformance expectations and baseline evidence. Existing custom content types suffice; no public Go API or module dependency change is planned. Existing provider fixture inputs remain unchanged; the new mid-output fallback input is imported byte-identically from the selected Anthropic source.
