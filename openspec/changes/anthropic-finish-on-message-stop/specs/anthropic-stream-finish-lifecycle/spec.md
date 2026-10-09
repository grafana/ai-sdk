## Purpose

Define when the Anthropic streaming adapter emits `PartFinish`, so direct `DoStream` callers and the AI Gateway observe the same finish contract as the registered upstream `@ai-sdk/anthropic`.

## ADDED Requirements

### Requirement: One finish per completed message

The Anthropic stream adapter SHALL emit exactly one `PartFinish` for each `message_stop`, and no `PartFinish` for any other event. The finish SHALL carry the finish reason, usage and provider metadata accumulated for that message.

#### Scenario: Several deltas, one finish
- **WHEN** one message contains three `message_delta` events followed by `message_stop`
- **THEN** the stream SHALL contain exactly one `PartFinish`, produced after the `message_stop` is handled
- **AND** it SHALL carry the last delta's finish reason and the latest usage

#### Scenario: Message without a delta
- **WHEN** a message has `message_start` with prepopulated tool calls and no `message_delta`, followed by `message_stop`
- **THEN** the stream SHALL contain one `PartFinish` for that message
- **AND** its finish reason SHALL be the latest non-null `stop_reason` seen on `message_start` or `message_delta` in the stream, or `other` with an empty raw value when none was seen

#### Scenario: Usage on a message without a delta
- **WHEN** a message without a `message_delta` follows a message that had one
- **THEN** its finish SHALL carry the output usage of the latest delta, because `message_start` sets only input and cache counters

#### Scenario: Consecutive messages
- **WHEN** a stream contains N complete `message_start` ... `message_stop` pairs
- **THEN** it SHALL contain N `PartFinish` parts, one after each corresponding `message_stop`

#### Scenario: Imported upstream fixture
- **WHEN** the unchanged upstream `programmatic-tool-calling` fixture (15 message pairs, 2 deltas) is replayed
- **THEN** the stream SHALL contain 15 `PartFinish` parts

### Requirement: Raw part ordering around finish

With raw chunks requested, the adapter SHALL emit each frame's raw part before any part produced by handling that frame, so the raw `message_stop` precedes its `PartFinish`.

#### Scenario: Raw order for a normal message
- **WHEN** a message with a `message_delta` and `message_stop` is streamed with raw chunks enabled
- **THEN** the parts SHALL appear in the order raw `message_delta`, raw `message_stop`, `PartFinish`

### Requirement: No finish without message_stop

The adapter SHALL NOT synthesize a `PartFinish` for a message that has not received `message_stop`.

#### Scenario: End of stream after delta
- **WHEN** the response body ends after a `message_delta` without `message_stop`
- **THEN** the stream SHALL contain no `PartFinish` for that message

#### Scenario: Overlapping messages remain invalid
- **WHEN** a `message_start` for a different message ID arrives while another message is open
- **THEN** the stream SHALL emit an error part and no `PartFinish` for either message

### Requirement: Error frames do not end the stream

An Anthropic `error` frame received after the first frame SHALL produce an error part and the adapter SHALL keep reading. Transport and event-decoding failures SHALL remain terminal. An `error` as the first frame SHALL fail the call before a stream is returned.

#### Scenario: Error between delta and stop
- **WHEN** an `error` frame follows a `message_delta` and `message_stop` follows the error
- **THEN** the stream SHALL contain the error part followed by one `PartFinish` for that message

#### Scenario: Error without stop
- **WHEN** an `error` frame follows a `message_delta` and the body then ends
- **THEN** the stream SHALL contain the error part and no `PartFinish`

#### Scenario: Error as first frame
- **WHEN** the first frame of the response is an `error` frame
- **THEN** the call SHALL fail with an API call error and SHALL NOT return a stream

#### Scenario: Transport failure stays terminal
- **WHEN** the connection fails or a frame cannot be decoded
- **THEN** the stream SHALL emit an error part and stop consuming
