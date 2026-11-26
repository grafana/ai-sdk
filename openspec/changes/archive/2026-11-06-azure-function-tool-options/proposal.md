## Why

Azure-identified Responses models discard function-tool options supplied under `azure`. This drops deferred loading and changes history encoding for tools with output schemas. Issue #630 requires the registered upstream namespace precedence.

## What Changes

- Read the model identity's function-tool namespace first, falling back to `openai` only when the selected object is absent or null.
- Apply that selection to request declarations and output-schema history conversion.
- Document attaching the existing typed options directly under the `azure` map key; no new public type is needed.

## Capabilities

### Modified Capabilities
- `openai-responses-provider`: function-tool namespace selection and synthetic HTTP regression evidence.

## Impact

Touches OpenAI Responses conversion, tests and provider guidance. No dependency, protocol or frontend changes.

## Non-goals

No Azure factory, new namespace abstraction, custom-provider-tool option changes, live recording, or changes to legacy call-level option resolution.
