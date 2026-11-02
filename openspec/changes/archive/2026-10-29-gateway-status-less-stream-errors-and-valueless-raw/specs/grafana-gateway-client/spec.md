## MODIFIED Requirements

### Requirement: Registered stream normalization

The client SHALL ignore an SSE data payload exactly equal to `[DONE]`, SHALL treat transport clean EOF as clean completion with or without a preceding finish, SHALL filter `raw` parts unless `IncludeRawChunks` is true, SHALL convert valid response-metadata timestamps into `time.Time`, and SHALL preserve response order. Context cancellation SHALL end the stream without manufacturing a provider error.

#### Scenario: Native stream identity is not a public route
- **WHEN** native id/modelId contain valid Unicode or punctuation outside route-ID syntax
- **THEN** those values SHALL be retained without route validation or normalization

#### Scenario: Response metadata is partial or empty
- **WHEN** metadata supplies no identity, only one field or explicit empty optional strings
- **THEN** the client SHALL decode only the supplied values with Go zero-value normalization and no canonical fallback

#### Scenario: Response metadata is malformed
- **WHEN** an optional identity field is null/wrong-type, timestamp is invalid or the bounded original document has malformed UTF-8
- **THEN** the client SHALL emit at most one bounded protocol PartError and close without returning substituted identity

#### Scenario: DONE terminates no value
- **WHEN** the stream contains exact `[DONE]`
- **THEN** the sentinel SHALL not appear as a `provider.StreamPart` and subsequent clean EOF SHALL be accepted

#### Scenario: Response timestamp is present
- **WHEN** response metadata contains a valid registered timestamp string
- **THEN** the corresponding `provider.StreamPart.Timestamp` SHALL represent the same instant

#### Scenario: Raw output is not requested
- **WHEN** a valid raw part arrives and `IncludeRawChunks` is false or omitted
- **THEN** the raw part SHALL be consumed but not delivered

#### Scenario: Raw output is requested
- **WHEN** a valid raw part arrives and `IncludeRawChunks` is true
- **THEN** the bounded raw value SHALL be delivered unchanged

#### Scenario: Raw part has no value
- **WHEN** the event `{"type":"raw"}` with no `rawValue` member arrives
- **THEN** it SHALL be a valid raw part rather than a protocol error
- **AND** it SHALL be delivered in order as a `PartRaw` with a nil `RawValue` when `IncludeRawChunks` is true, and consumed but not delivered otherwise

#### Scenario: Clean EOF occurs without finish
- **WHEN** a valid stream or `[DONE]` is followed by transport EOF before any finish
- **THEN** the client SHALL close cleanly to match the registered client's observable EOF behavior
- **AND** this SHALL include EOF directly after a stream error event, with no further part manufactured

## ADDED Requirements

### Requirement: Status-less stream error decoding

The client SHALL accept an SSE error event without `statusCode` only when its type is `internal_server_error` and its code is `provider_error`, and SHALL require `retryable` to be present and false. That event SHALL become an ordered `PartError` whose `APICallError` has status 0, is not retryable, carries the public message, and retains supplied `error.data` exactly. Every other registered code SHALL still require its registered status.

#### Scenario: Status-less provider error is received
- **WHEN** a stream contains `{"type":"error","error":{"message":"provider stream error","type":"internal_server_error","param":null,"code":"provider_error","retryable":false}}` followed by valid parts
- **THEN** the client SHALL deliver a `PartError` with `APICallError.StatusCode` 0, `IsRetryable` false and message `provider stream error`, and then deliver the later parts in order

#### Scenario: Status-less provider error has a null status
- **WHEN** the `provider_error` document carries `"statusCode": null`
- **THEN** the client SHALL treat the status as absent under ordinary Go decoding and decode the event as a status-less provider error

#### Scenario: Status-less document is inconsistent
- **WHEN** the `provider_error` document carries an integer `statusCode`, omits `retryable`, or sets `retryable` to true
- **THEN** the client SHALL emit one terminal non-retryable protocol PartError and close

#### Scenario: Another code omits its status
- **WHEN** an SSE error event with any other registered code, such as `upstream_error`, omits `statusCode` or sets it to `null`
- **THEN** the client SHALL emit one terminal non-retryable protocol PartError and close

#### Scenario: HTTP error uses the stream-only code
- **WHEN** a non-2xx HTTP response carries the `provider_error` code
- **THEN** the client SHALL NOT accept it as a registered category and SHALL return the bounded local protocol error
