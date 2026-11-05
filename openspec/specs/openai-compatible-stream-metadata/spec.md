# openai-compatible-stream-metadata Specification

## Purpose

Define when OpenAI-compatible responses expose meaningful identity and timestamp
metadata rather than provider placeholder values.

## Requirements

### Requirement: Placeholder chunks do not consume response metadata
The OpenAI-compatible provider SHALL wait for the first chunk containing a
nonempty response ID, model ID or nonzero creation timestamp before emitting
response metadata. It SHALL emit metadata at most once. Zero creation timestamps
SHALL be treated as placeholders in both unary and streaming responses.

#### Scenario: Empty placeholder precedes metadata
- **WHEN** a provider emits an empty-ID/model, zero-time placeholder before a chunk containing real metadata
- **THEN** the response metadata part contains the later ID, model and timestamp
- **AND** no placeholder metadata part is emitted

#### Scenario: Unary timestamp is zero
- **WHEN** a unary response has created equal to zero
- **THEN** the normalized timestamp is absent rather than the Unix epoch

### Requirement: Tool input parts are identified by id

OpenAI-compatible streaming tool calls SHALL emit `tool-input-start`, `tool-input-delta` and `tool-input-end` parts identified by `id`. Only `tool-input-start` SHALL carry the tool name. None of them SHALL carry a separate tool call ID field.

#### Scenario: Streamed tool call
- **WHEN** a streamed tool call produces input parts
- **THEN** each part SHALL carry the call `id`, only the start part SHALL carry `toolName`, and the final `tool-call` part SHALL carry `toolCallId` and `toolName`

### Requirement: Opt-in Chat output metadata
The provider SHALL support a local `preserveChatCompletionMetadata` option that
preserves validated refusal, logprob and detailed token usage fields under
`openaiCompatible.chatCompletion`. It SHALL NOT forward this option to the native
API. This opt-in Go extension intentionally exceeds the registered
`@ai-sdk/openai-compatible` 3.0.82 normalization while leaving default behavior unchanged.

#### Scenario: Unary refusal with probabilities
- **WHEN** preservation is enabled and a native response includes refusal and logprobs
- **THEN** typed provider metadata retains both fields and rejects invalid probability or byte values
- **AND** unrelated native fields are not copied into that metadata

#### Scenario: Streaming metadata
- **WHEN** preservation is enabled for a native stream
- **THEN** refusal and logprob deltas appear on custom parts with kind `openai-compatible.chat-completion`
- **AND** final detailed usage appears on the finish part with absent counters remaining absent

#### Scenario: Default provider parity
- **WHEN** preservation is not enabled
- **THEN** no Chat metadata extension or additional custom parts are emitted
