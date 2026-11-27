## Why

OpenAI Responses reports when it transforms input items, such as dropping preserved reasoning after the prompt changes. Callers cannot distinguish no reported transformations from a dropped item because the adapter omits the field.

## What Changes

- Expose `input_transformations` as `inputTransformations` in the active provider metadata namespace of unary results and terminal stream parts.
- Preserve explicit empty arrays and open string values; omit null and missing arrays.
- Apply the same behavior to Mantle through the shared adapter. Batch, cache diagnostics and UI message framing are out of scope.

## Capabilities

### Modified Capabilities

- `openai-responses-provider`: Return input transformation metadata.

## Impact

The existing Responses adapter gains an optional provider metadata field. No request, transport, or public Go type changes.
