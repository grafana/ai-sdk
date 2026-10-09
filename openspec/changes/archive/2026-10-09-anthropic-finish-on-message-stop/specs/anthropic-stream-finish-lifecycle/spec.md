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

An Anthropic `error` frame received after the first frame SHALL produce an error part and the adapter SHALL keep reading. An `error` frame whose body cannot be decoded SHALL still produce an error part and reading SHALL continue. Transport failures and failures to decode any other event SHALL remain terminal. An `error` as the first frame SHALL fail the call before a stream is returned.

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
- **WHEN** the connection fails or a non-error event cannot be decoded
- **THEN** the stream SHALL emit an error part and stop consuming

#### Scenario: Undecodable error frame
- **WHEN** an `error` frame body cannot be decoded
- **THEN** the stream SHALL contain an error part and continue reading

### Requirement: Error frames follow upstream's classification

An Anthropic `error` frame SHALL produce an error part, or fail the call when it is the first frame, carrying the inner error message and the status code and retryability upstream assigns to its error type.

#### Scenario: Classified error types
- **WHEN** an error frame has type `api_error`, `overloaded_error`, `rate_limit_error`, `request_too_large`, `authentication_error`, `permission_error`, `not_found_error`, `billing_error` or `invalid_request_error`
- **THEN** the status SHALL be 500, 529, 429, 413, 401, 403, 404, 400 or 400 respectively
- **AND** only `api_error`, `overloaded_error` and `rate_limit_error` SHALL be retryable

#### Scenario: Rate limit error part
- **WHEN** an `error` frame of type `rate_limit_error` arrives after the stream started
- **THEN** the error part SHALL have status 429, SHALL be retryable, and SHALL carry the inner message

#### Scenario: Unclassified first frame
- **WHEN** the first frame is an `error` of an unclassified type
- **THEN** the call SHALL fail with status 500 and SHALL NOT be retryable

#### Scenario: Unclassified error part
- **WHEN** an error frame of an unclassified type arrives after the stream started
- **THEN** the error part SHALL have no status and SHALL NOT be retryable

### Requirement: Raw usage iterations keep declared fields

The raw usage retained in finish usage and provider metadata SHALL keep, for each iteration, only `type`, `model`, `input_tokens`, `output_tokens`, `cache_creation_input_tokens` and `cache_read_input_tokens`, as upstream's response schema does.

#### Scenario: Nested cache creation breakdown
- **WHEN** a usage iteration contains a nested `cache_creation` object
- **THEN** the retained raw usage iteration SHALL NOT contain it

### Requirement: Provider metadata is stream-level

Finish provider metadata SHALL be built from state that lives for the whole stream, as upstream does. A `message_delta` replaces the stop sequence, stop details and container, and replaces context management and the safeguard verdict only when it supplies a non-null value. Usage iterations reported in the metadata follow the same stream-level state that the token totals use, while raw usage restarts at each `message_start`. `message_start` contributes only a non-null container.

#### Scenario: Delta that omits the verdict and stop sequence
- **WHEN** a second message's delta has `stop_sequence: null` and `safeguard_results: null` after a first message that had a stop sequence and a verdict
- **THEN** the second finish SHALL have no stop sequence and SHALL carry the first message's verdict

#### Scenario: Message without a delta
- **WHEN** a message has no `message_delta` after a message that did
- **THEN** its finish SHALL repeat the previous stop sequence, verdict, context management and iterations

#### Scenario: Container on message start
- **WHEN** `message_start` carries a container and no delta replaces it
- **THEN** the finish SHALL report that container

#### Scenario: Null context management on a delta
- **WHEN** a later `message_delta` has `context_management: null`
- **THEN** the finish SHALL keep the earlier context management

#### Scenario: Iterations on a later message
- **WHEN** a second message's delta reports no `iterations` after a first message's delta did
- **THEN** the second finish SHALL repeat the first message's iterations in provider metadata and in the token totals
- **AND** its raw `usage` metadata SHALL contain only the second message's usage
