## Why

Decoding malformed UI history currently turns missing required fields into valid-looking zero values. For example, a missing document citation title becomes an empty string and the Agent may start a provider stream.

## What Changes

- Require the pinned UI message and represented part fields at JSON decode, including nested citations and tool approval fields.
- Preserve valid empty strings, supported opaque null payloads, and ordinary Go JSON key handling.
- Verify the rejection boundary against the pinned TypeScript validator and a deterministic Agent HTTP endpoint.

## Capabilities

### Modified Capabilities

- `ui-message-conversion`: Specify required-field validation at the UI JSON boundary.

## Impact

No new public API, provider or SSE codec changes, or additional validation for Go-built messages. Unrepresented part kinds, application schemas, and broader optional field and semantic parity are outside this change.
