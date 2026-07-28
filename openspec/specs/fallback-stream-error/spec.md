## Purpose

Define fallback behavior when a candidate stream's first chunk is an error, including candidate selection, replay, cleanup, decider handling, and cancellation.

## Requirements

### Requirement: First-chunk error detection in DoStream
`fallback.Model.DoStream` SHALL read the first chunk from each candidate's stream before returning. If the first chunk is a `PartError`, the fallback SHALL treat it as a synchronous error and try the next candidate according to the decider policy.

#### Scenario: Primary streams PartError, secondary succeeds
- **WHEN** the primary candidate's `DoStream` returns `(result, nil)` and the stream's first chunk is a `PartError`
- **THEN** fallback SHALL try the secondary candidate and return its stream

#### Scenario: Primary streams PartError, decider rejects fallback
- **WHEN** the primary candidate's stream emits a `PartError` as first chunk and the decider returns `false` for that error
- **THEN** fallback SHALL return the error without trying further candidates

#### Scenario: All candidates stream PartError
- **WHEN** all candidates' streams emit `PartError` as their first chunk
- **THEN** fallback SHALL return the last candidate's error

### Requirement: Valid first chunk replay
When the first chunk from a candidate's stream is not a `PartError`, `fallback.Model.DoStream` SHALL return a stream that emits the peeked chunk first, followed by all remaining chunks from the original stream in order.

#### Scenario: Primary succeeds with valid first chunk
- **WHEN** the primary candidate's stream emits a valid `PartTextDelta` as its first chunk
- **THEN** fallback SHALL return a stream whose first event is that `PartTextDelta`, followed by all subsequent events in original order

#### Scenario: Primary stream is empty
- **WHEN** the primary candidate's `DoStream` returns `(result, nil)` and the stream channel is immediately closed
- **THEN** fallback SHALL return a closed, empty stream without error

### Requirement: Failed stream cleanup
When a candidate's stream emits a `PartError` as its first chunk, `fallback.Model.DoStream` SHALL drain the remaining stream to prevent goroutine leaks in the candidate's producer.

#### Scenario: Stream drained after PartError
- **WHEN** a candidate's stream emits a `PartError` followed by additional events before closing
- **THEN** fallback SHALL consume all remaining events from that stream

### Requirement: Decider applied to stream errors
Stream `PartError` errors SHALL be subject to the same decider function as synchronous errors. The decider determines whether to fall back or stop.

#### Scenario: Stream error with context length message
- **WHEN** a candidate's stream emits a `PartError` whose error message contains "context length"
- **THEN** the default decider SHALL reject fallback and the error SHALL be returned immediately

#### Scenario: Stream error with generic message
- **WHEN** a candidate's stream emits a `PartError` whose error message is "model not found"
- **THEN** the default decider SHALL allow fallback to the next candidate

### Requirement: Context cancellation during stream peek
If the context is cancelled while waiting for the first chunk from a candidate's stream, `fallback.Model.DoStream` SHALL respect cancellation and return the context error.

#### Scenario: Context cancelled during first chunk read
- **WHEN** the context is cancelled while `DoStream` is waiting for the first chunk from a candidate's stream
- **THEN** fallback SHALL return the context error
