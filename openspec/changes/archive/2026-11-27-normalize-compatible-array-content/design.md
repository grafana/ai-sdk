## Context

The registered `@ai-sdk/openai-compatible@3.0.94` tag (`3dad57f204c7332178d28c3467c7311875b66a3e`) uses one loose content schema and converter in both Chat response paths. Its tests cover text, thinking and unknown blocks. The Go adapter currently decodes both content fields as strings.

## Decisions

- Decode both shapes with one response-only type. Validate that each outer array member is an object with a string discriminator; ignore unrecognized discriminators and malformed optional text or thinking payloads.
- In newly accepted arrays, use the pin's scalar reasoning selection and placement, then walk the recognized blocks in order. Preserve existing scalar-only selection and unary ordering.
- Use the same stream lifecycle for reasoning from a thinking block or a scalar field: end the active text part before emitting reasoning, and vice versa. This also splits previously overlapping parts when a scalar-only stream sends reasoning after text.
- Keep array content before refusal and tools. Keep raw body, metadata, usage and error handling on their existing paths.

## Evidence

Synthetic public model calls verify the conversion and rejection/recovery cases. A Go test server and pinned TypeScript parser and message assembler verify UI order. The pin's array cases are inline test inputs, not provider captures. No recorded or imported fixture covers array content; no live acceptance is claimed.
