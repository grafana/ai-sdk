## Why

`DataPart.PartType()` returns `"data"` and `ToolInvocationPart.PartType()` returns `"tool-invocation"`, but the JSON wire format uses `"data-{DataName}"` and `"tool-{toolName}"` respectively. This makes `PartType()` inconsistent with the serialized form and diverges from the upstream TypeScript SDK where the `type` field matches the wire discriminator (e.g., `data-usage`, `tool-searchWeb`). Since `PartType()` is part of the public `Part` interface, returning a value that doesn't match the wire type is misleading to consumers.

## What Changes

- `DataPart.PartType()` will return `"data-" + p.DataName` instead of `"data"`, matching the wire format.
- `ToolInvocationPart.PartType()` will return `"tool-" + p.ToolName` instead of `"tool-invocation"`, matching the wire format.
- The `UIPartData` and `UIPartToolInvocation` constants will be documented as base prefixes, not exact `PartType()` return values.
- Tests will be updated to assert the new wire-compatible return values.

## Capabilities

### New Capabilities

- `part-type-wire-compat`: Ensure `PartType()` returns wire-compatible type strings for all Part implementations, matching the JSON `type` discriminator used in serialization.

### Modified Capabilities

_(none -- no existing spec-level requirements change; this is a new behavioral specification)_

## Impact

- **Code**: `message.go` (PartType methods), `message_json_test.go` and `http_test.go` (test assertions that compare PartType values).
- **API**: `Part.PartType()` return values change for `DataPart` and `ToolInvocationPart`. Callers that compare `PartType()` against the old fixed strings `"data"` or `"tool-invocation"` will need updating. However, as noted in the issue, all existing internal consumers use Go type switches (not `PartType()` string comparisons), so practical breakage is minimal.
- **Wire format**: No change -- serialization already uses the correct `"data-{name}"` / `"tool-{name}"` format.
