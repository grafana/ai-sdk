## Why

OpenAI Responses currently forwards JSON response and function-tool schemas without the normalization required by the registered upstream `@ai-sdk/openai` 4.0.71 behavior. A string `propertyNames` constraint can therefore reach OpenAI despite being unsupported there; non-null, non-string forms are not rejected before a request, and callers receive no compatibility warning.

## What Changes

- Normalize JSON response schemas and function-tool input and optional output schemas before constructing OpenAI Responses requests, including namespaced function tools.
- Remove supported string-schema `propertyNames` recursively with a compatibility warning; remove `propertyNames: null` without warning; reject non-null, non-string or boolean `propertyNames`; leave unrelated schema keywords intact.
- Keep caller-owned schemas immutable and use the same conversion for generate and stream requests, regardless of the `strictJsonSchema` setting.
- Add focused provider tests for emitted requests, warnings, local failures without HTTP requests, and upstream traversal cases.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: Clarify schema normalization and failure behavior for structured response and function-tool requests.

## Impact

The provider-private request builders in `providers/openai/apply_options.go`, `providers/openai/prepare_tools.go`, and `providers/openai/convert_request.go` and their focused tests change. No public API, non-OpenAI provider, generic Go schema generator, or frontend wire protocol changes are proposed. This independent provider parity package targets the pinned upstream commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d` (`@ai-sdk/openai` 4.0.71), not a baseline upgrade. Provider conformance inputs must not be synthesized to support the change.
