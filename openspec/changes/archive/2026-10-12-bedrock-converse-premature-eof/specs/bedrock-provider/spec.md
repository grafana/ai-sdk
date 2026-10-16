## MODIFIED Requirements

### Requirement: Streaming response decoding via Smithy event stream

For `DoStream`, the provider SHALL decode the AWS Smithy event-stream binary response, extracting `(:event-type, JSON payload)` pairs from each frame, and emit corresponding `provider.StreamPart` values on a buffered channel.

#### Scenario: Channel buffer size

- **WHEN** `DoStream` returns a `StreamResult`
- **THEN** the channel buffer is at least 64 elements

#### Scenario: Text delta

- **WHEN** the stream emits `contentBlockDelta` with `delta.text = "foo"` at index `0`
- **THEN** the channel receives a `PartTextDelta` with `Delta: "foo"` and ID derived from block index `0`

#### Scenario: Tool input streaming

- **WHEN** the stream emits `contentBlockStart` with `toolUse{toolUseId, name}` followed by one or more `contentBlockDelta` with `toolUse.input` fragments and a final `contentBlockStop`
- **THEN** the channel receives `PartToolInputStart`, `PartToolInputDelta` for each fragment, and `PartToolInputEnd` carrying the accumulated JSON

#### Scenario: Reasoning delta

- **WHEN** the stream emits `contentBlockDelta` with `delta.reasoningContent.text`
- **THEN** the channel receives a `PartReasoningDelta` with the text fragment

#### Scenario: Finish reason and usage

- **WHEN** the stream emits `messageStop` with `stopReason` followed by `metadata` with `usage`
- **THEN** the channel emits a finish part carrying the mapped unified finish reason and a usage part built from the metadata

#### Scenario: Clean EOF before messageStop

- **WHEN** the response body ends on a complete frame boundary before a `messageStop` event, including an empty body or a stream containing metadata only
- **THEN** the channel emits exactly one final `PartError` carrying a retryable `*provider.APICallError` whose message states that the stream ended before `messageStop`
- **AND** the channel closes without a `PartFinish` or a synthesized `PartToolCall` for an unclosed tool block
- **AND** this intentionally differs from the registered `@ai-sdk/amazon-bedrock` 5.0.105 behavior, which emits a finish with `other` and an absent raw reason on clean EOF without `messageStop`

#### Scenario: Clean EOF after messageStop

- **WHEN** the response body ends after `messageStop`, with or without a subsequent `metadata` event
- **THEN** the channel emits one finish carrying the mapped reason and any reported usage
- **AND** an empty stop reason or an unmapped reason such as `model_context_window_exceeded` remains `other` with the received raw reason and does not become a premature-EOF error

#### Scenario: Stream-level exception

- **WHEN** the stream emits a frame whose `:exception-type` is `throttlingException`
- **THEN** the channel receives a `PartError` carrying a `*provider.APICallError` with `IsRetryable = true`, and is then closed

#### Scenario: Mid-stream transport failure

- **WHEN** the stream fails after a 2xx response has begun
- **THEN** the channel emits a final `PartError` with a synthesized retryable `*provider.APICallError` and is then closed

#### Scenario: Context cancellation

- **WHEN** the call's context is cancelled mid-stream
- **THEN** the provider cancels the underlying HTTP request and closes the channel without panicking
