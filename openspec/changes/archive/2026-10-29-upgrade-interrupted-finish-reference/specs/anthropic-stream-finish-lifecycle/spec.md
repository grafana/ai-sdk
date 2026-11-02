# Spec Delta

## MODIFIED Requirements

### Requirement: One finish per completed message

The Anthropic stream adapter SHALL emit exactly one `PartFinish` for each `message_stop`, and no `PartFinish` for any other event. The finish SHALL carry the finish reason, usage and provider metadata accumulated for that message. The only other finish is the end-of-stream interrupted finish defined below.

#### Scenario: Several deltas, one finish
- **WHEN** one message contains three `message_delta` events followed by `message_stop`
- **THEN** the stream SHALL contain exactly one `PartFinish`, produced after the `message_stop` is handled
- **AND** it SHALL carry the last delta's finish reason and the latest usage

#### Scenario: Message without a delta
- **WHEN** a message has `message_start` with prepopulated tool calls and no `message_delta`, followed by `message_stop`
- **THEN** the stream SHALL contain one `PartFinish` for that message
- **AND** its finish reason SHALL come from the latest `message_delta` (a null `stop_reason` there maps to `other` with an empty raw value) or from a non-null `message_start.stop_reason`, whichever came last, or `other` with an empty raw value when none was seen

#### Scenario: Usage on a message without a delta
- **WHEN** a message without a `message_delta` follows a message that had one
- **THEN** its finish SHALL carry the output usage of the latest delta, because `message_start` sets only input and cache counters

#### Scenario: Consecutive messages
- **WHEN** a stream contains N complete `message_start` ... `message_stop` pairs
- **THEN** it SHALL contain N `PartFinish` parts, one after each corresponding `message_stop`

#### Scenario: Imported upstream fixture
- **WHEN** the unchanged upstream `programmatic-tool-calling` fixture (15 message pairs, 2 deltas) is replayed
- **THEN** the stream SHALL contain 15 `PartFinish` parts

## REMOVED Requirements

### Requirement: No finish without message_stop
**Reason**: The registered upstream reference reports a stream cut off after output as an `interrupted` finish instead of ending without a finish.
**Migration**: See "Interrupted finish after partial output". Overlapping messages remain invalid and still produce an error part without a finish.

## ADDED Requirements

### Requirement: Interrupted finish after partial output

When the response body ends cleanly after output since the last `response-metadata` or `PartFinish`, and no error part was emitted, the adapter SHALL append one `PartFinish` with unified reason `interrupted` and no raw reason. Output is a non-empty text, reasoning or tool-input delta, or a tool call. The finish SHALL repeat the previous finish's usage and provider metadata, or carry empty usage. Cancellation SHALL NOT produce it.

#### Scenario: End of stream after partial text
- **WHEN** the body ends after a non-empty `text_delta` without `message_stop`
- **THEN** the stream SHALL end with one `PartFinish` whose unified reason is `interrupted`, with empty usage

#### Scenario: End of stream after a delta without message_stop
- **WHEN** the body ends after `message_delta` and before `message_stop`
- **THEN** the stream SHALL end with an `interrupted` finish, not the delta's finish reason

#### Scenario: Interrupted later message
- **WHEN** a completed message is followed by a message cut off after output
- **THEN** the stream SHALL contain the completed message's finish and an `interrupted` finish repeating its usage and provider metadata

#### Scenario: No output since the last boundary
- **WHEN** the body ends after only `message_start`, empty deltas, or a new `message_start` following a completed message
- **THEN** no interrupted finish SHALL be emitted and existing empty-stream handling SHALL apply

#### Scenario: Errors keep their path
- **WHEN** an error part was emitted anywhere in the stream before the body ends
- **THEN** no interrupted finish SHALL be emitted

#### Scenario: Overlapping messages remain invalid
- **WHEN** a `message_start` for a different message ID arrives while another message is open
- **THEN** the stream SHALL emit an error part and no `PartFinish` for either message
