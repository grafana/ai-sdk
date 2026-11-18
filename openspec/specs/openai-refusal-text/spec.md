# openai-refusal-text Specification

## Purpose

Define how the OpenAI Responses and OpenAI-compatible Chat Completions
providers surface a model refusal, so a refused answer reaches the caller as
text with a finish reason that reports it.

## Requirements

### Requirement: Responses refusals are text
The OpenAI Responses provider SHALL convert a message `refusal` content part to
a text part with the message's item ID and phase metadata, and SHALL stream
`response.refusal.delta` events as text deltas of the message's text part. A
response without an incomplete reason that contains a refusal SHALL finish with
`content-filter`, unless it also contains a client-side function call, which
SHALL finish with `tool-calls` so its tools still run. Incomplete reasons keep
their existing mapping.

#### Scenario: Refused Responses answer
- **WHEN** a generated or streamed Responses message contains only a refusal
- **THEN** the refusal text is returned as text with the message item ID
- **AND** the finish reason is `content-filter`

#### Scenario: Refusal with a function call
- **WHEN** a Responses output contains a refusal and a client-side function call
- **THEN** the finish reason is `tool-calls`

### Requirement: Chat Completions refusals are text
The OpenAI-compatible provider SHALL return a non-empty string `refusal` as a
text part after the message content, and SHALL stream refusal deltas into the
same text part as content deltas. A refusal SHALL turn a `stop` finish reason
into `content-filter` while keeping `stop` as the raw reason, including when the
refusal arrives in the chunk that carries the finish reason. Other finish
reasons SHALL keep their mapping. Non-string refusal values SHALL be ignored
instead of failing the response.

#### Scenario: Refusal in the finish chunk
- **WHEN** a stream sends content, then a refusal delta together with `finish_reason: "stop"`
- **THEN** one text part carries the content and the refusal
- **AND** the finish reason is `content-filter` with raw reason `stop`

#### Scenario: Non-string refusal
- **WHEN** a compatible server sends `refusal: false`
- **THEN** the response is converted without a refusal and its finish reason is unchanged
