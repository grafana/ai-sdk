# openai-compatible-stream-termination Specification

## Purpose

Define how the OpenAI-compatible provider ends a streamed response that never
reported a finish reason.

## Requirements

### Requirement: A stream without a finish reason ends in error

The OpenAI-compatible provider SHALL report a streamed response that ends without a `finish_reason` as an error, whether the body ends at EOF, sends `[DONE]` without one, or is not an SSE stream at all. It SHALL emit an error part with the message `openai: response stream ended without a finish reason` and SHALL end the stream with a `finish` part whose unified finish reason is `error`. A stream that reported a finish reason, or whose finish reason an earlier error already set, SHALL NOT receive this additional error.

#### Scenario: Stream cut before the finish reason

- **WHEN** a streamed response delivers text deltas and closes without any chunk carrying `finish_reason`
- **THEN** the stream contains one error part with that message and ends with finish reason `error`
- **AND** `GenerateText` returns an error rather than the partial text as a successful result

#### Scenario: HTTP 200 body that is not an event stream

- **WHEN** the provider answers a streaming request with HTTP 200 and an HTML body
- **THEN** the stream contains the same error part and ends with finish reason `error`

#### Scenario: Normal completion is unaffected

- **WHEN** a chunk carries `finish_reason: "stop"` before the stream closes
- **THEN** no additional error part is emitted and the finish reason is `stop`
