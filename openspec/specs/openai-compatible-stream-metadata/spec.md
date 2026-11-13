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
The provider SHALL offer constructor option `WithChatCompletionMetadata()` to
retain validated first-choice refusal and logprobs under `chatCompletion` in the
resolved provider metadata namespace. Client request options SHALL NOT activate
this extension or be rewritten to enable it. Native usage SHALL remain in
`Usage.Raw`. This opt-in Go extension exceeds registered compatible normalization.

#### Scenario: Unary refusal with probabilities
- **WHEN** the model enables preservation and the first choice contains refusal and logprobs
- **THEN** provider metadata SHALL retain both alongside existing namespace fields
- **AND** empty logprob objects and unrelated fields SHALL be omitted

#### Scenario: Streaming metadata
- **WHEN** an enabled stream sends refusal fragments and logprobs across chunks
- **THEN** the finish part SHALL contain concatenated refusal and appended logprob channels
- **AND** no custom content part SHALL be emitted or stored in message history

#### Scenario: Malformed optional metadata
- **WHEN** first-choice metadata has an invalid type, probability or byte value
- **THEN** the extension for that response or chunk SHALL be dropped with a unary warning or non-retryable recoverable stream error
- **AND** valid content and subsequent chunks SHALL continue to be processed

#### Scenario: Unused data does not invalidate metadata
- **WHEN** non-first choices contain malformed metadata or content has invalid UTF-8 bytes
- **THEN** first-choice metadata SHALL remain available with ordinary Go JSON decoding

#### Scenario: Default provider parity
- **WHEN** preservation is not enabled in the constructor
- **THEN** no Chat metadata extension SHALL be emitted and provider options SHALL retain existing passthrough behavior
