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

### Requirement: Native audio preservation
With Chat output preservation enabled, the provider SHALL retain native audio
identity, expiry, base64 bytes and transcript in typed `chatCompletion.audio`
metadata. Unary audio SHALL contain all four fields; streaming audio SHALL retain
field presence in each delta. Invalid base64 or negative expiry SHALL fail decoding.
The provider SHALL accept typed native output modalities/audio options and an
assistant audio reference containing only its native ID.

#### Scenario: Audio stream with separate metadata and bytes
- **WHEN** a native stream sends audio identity, bytes/transcript and expiry in separate chunks
- **THEN** three typed custom metadata parts retain those fields without invented empty values

#### Scenario: Audio history continuation
- **WHEN** an assistant message carries a valid native audio reference and no text
- **THEN** the native request contains its audio ID and null content
- **AND** audio bytes or unrelated fields are not accepted in the history reference

### Requirement: Custom text tool transport
The compatible provider SHALL map provider tool ID `openai.custom` into native
custom declarations and named choices. It SHALL emit text-encoded streamed input
and canonical JSON-string final calls, matching the existing Responses domain
representation. A local custom history marker SHALL preserve type without
requiring a current declaration or overriding common identity.

#### Scenario: Grammar-constrained tool
- **WHEN** a custom tool declares a native text or grammar format
- **THEN** the provider retains its declaration and text input without wrapping it as a function argument object

#### Scenario: Stream encoding remains stable
- **WHEN** a native tool call changes between function and custom encodings during its stream
- **THEN** the provider emits an error instead of reinterpreting previous input fragments

### Requirement: Native prediction and search parameters
The provider SHALL validate typed native prediction and web-search controls
before sending them. Prediction content SHALL be text or nonempty text parts;
unknown prediction part properties SHALL fail validation.

#### Scenario: Static prediction with an explicit breakpoint
- **WHEN** predicted text parts include a valid explicit cache breakpoint
- **THEN** the native prediction retains text and breakpoint values exactly

#### Scenario: Invalid prediction union
- **WHEN** prediction content is null, empty parts, or a non-text part
- **THEN** request construction fails before network I/O

### Requirement: Native inline moderation
The provider SHALL forward validated moderation configuration and, when Chat
metadata preservation is enabled, retain typed native input/output outcomes.
Scores SHALL be finite probabilities and result/error unions SHALL be validated.

#### Scenario: Moderation accompanies a streamed completion
- **WHEN** a native chunk contains valid input/output moderation outcomes
- **THEN** the provider emits a typed Chat metadata custom part with both outcomes

#### Scenario: Invalid moderation score
- **WHEN** native moderation reports a score outside zero through one
- **THEN** decoding fails instead of publishing invalid metadata

### Requirement: Exact native cache boundaries
The provider SHALL retain validated native cache options and content breakpoints.
Local typed text-boundary metadata SHALL only reconstruct arrays whose text
concatenation exactly equals the common message and SHALL not be sent upstream.

#### Scenario: Text boundary cannot change content or identity
- **WHEN** local cache text metadata changes text or includes a role or tool ID field
- **THEN** request construction fails before I/O

#### Scenario: Singleton user text boundary
- **WHEN** a single user text part has an explicit breakpoint
- **THEN** the provider retains the content array and its breakpoint

### Requirement: Extended token details
Opt-in Chat usage metadata SHALL preserve prompt cache-write, image and text
counts and completion text counts, retaining absent versus explicit zero.

#### Scenario: Invalid extended count
- **WHEN** any extended token detail is negative
- **THEN** metadata decoding fails before publishing the count

### Requirement: Fragmented native tool identity
With native Chat preservation enabled, indexed tool identity-only deltas SHALL
be accepted before or between function/custom input fragments. Input encoding
SHALL be established by the first declared type or payload and SHALL remain stable.

#### Scenario: Two choices split identity and custom input
- **WHEN** native choices reuse an ID but send indexed identity and custom input in separate chunks
- **THEN** each choice emits its own text-encoded tool input and canonical final JSON string
