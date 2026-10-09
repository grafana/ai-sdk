## ADDED Requirements

### Requirement: Bedrock raw stream chunks mirror upstream

When raw chunks are requested, each event-stream frame SHALL produce a raw part whose value is the frame's JSON payload without the AWS padding field `p`, wrapped under its event type, or under its exception type for exception frames. Payloads that are not JSON objects, or frames with no type, SHALL be forwarded unchanged.

#### Scenario: Event frame
- **WHEN** a `messageStop` event frame with payload `{"stopReason":"end_turn","p":"abc"}` is decoded with raw chunks requested
- **THEN** the raw value SHALL be `{"messageStop":{"stopReason":"end_turn"}}`

#### Scenario: Exception frame
- **WHEN** a `throttlingException` exception frame with payload `{"message":"rate limited"}` is decoded with raw chunks requested
- **THEN** the raw value SHALL be `{"throttlingException":{"message":"rate limited"}}`

### Requirement: Bedrock tool input parts are identified by id

`tool-input-start` SHALL carry the tool call `id` and the tool name, and SHALL NOT carry a separate tool call ID field.

#### Scenario: Tool use block
- **WHEN** a Converse tool-use block starts
- **THEN** the `tool-input-start` part SHALL carry `id` and `toolName` only
