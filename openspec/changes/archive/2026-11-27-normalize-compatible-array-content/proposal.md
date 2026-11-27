## Why

The OpenAI-compatible Chat adapter rejects an array in assistant response content, even though the registered upstream adapter recognizes text and thinking blocks in that form. A valid response can therefore fail generation or lose its streamed output.

## What changes

- Normalize array-valued `message.content` and `delta.content` in the outbound provider adapter.
- Preserve recognized block order and stream lifecycle boundaries. Reject invalid array envelopes while ignoring unsupported block payloads.
- Preserve scalar reasoning selection and scalar unary ordering. Stream boundaries also apply when scalar reasoning follows text. Request conversion, refusal handling, tool tracking and metadata are unchanged.

## Capabilities

### New Capabilities
- `openai-compatible-array-content`: Normalization and validation of array-valued assistant content.

## Impact

`providers/openai-compatible` and its Go and cross-language tests. No public API, new UI chunks or provider recording.
