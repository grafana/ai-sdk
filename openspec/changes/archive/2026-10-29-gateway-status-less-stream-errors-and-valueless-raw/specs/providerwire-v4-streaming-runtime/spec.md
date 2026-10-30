## MODIFIED Requirements

### Requirement: Ordered non-terminal provider errors

Each provider `PartError`, before or after a finish, SHALL be independently reduced through the closed safe-error classification and emitted in place with base fields `{"type":"error","error":{"message":string,"type":string,"param":null,"code":string,"statusCode":integer,"retryable":boolean}}`. The provider-stream category SHALL use its own fixed base fields with no `statusCode` member.

#### Scenario: Ordered non-terminal provider errors policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Optional `error.data` MAY carry native current-error and execution summaries under gateway-attempt-failure-evidence. A valid provider error SHALL not terminate the stream or alter lifecycle state; later metadata, content, additional provider errors, and finish SHALL remain valid under the sequential response segment rules.

#### Scenario: Provider error is followed by content
- **WHEN** a provider emits an error before or within a text block and later emits otherwise valid content and finish
- **THEN** the client SHALL receive the safe error and every later event in provider order

#### Scenario: Provider error follows a finish
- **WHEN** a provider emits an error after a valid finish and then either closes or emits response-metadata and another segment
- **THEN** the client SHALL receive the safe error in place, and the stream SHALL end at clean EOF or continue with the new segment

#### Scenario: Multiple provider errors are ordered
- **WHEN** a provider emits multiple errors separated by valid stream parts
- **THEN** each error SHALL be independently normalized and retained in its original position without terminating the stream

#### Scenario: Provider error contains hostile detail
- **WHEN** a provider error contains credentials, URLs, bodies, headers, data, causes, backend identity, or arbitrary messages
- **THEN** its base public fields SHALL retain the approved category message, while optional error data SHALL contain only the governed native summary/overview rather than a native error dump

#### Scenario: Provider error status is malformed
- **WHEN** an `APICallError` carries a non-zero status outside the valid HTTP range 100 through 599
- **THEN** it SHALL reduce to the canonical internal error without inspecting a wrapped transport cause

#### Scenario: Provider error has no HTTP status
- **WHEN** an `APICallError` has status zero and wraps a timeout-capable transport error
- **THEN** it SHALL reduce to the canonical timeout error
- **AND** a status-zero `APICallError` with an ordinary network or DNS cause SHALL reduce to the canonical upstream error
- **AND** a status-zero `APICallError` that wraps context cancellation or an expired deadline SHALL reduce to the canonical cancellation or timeout error respectively
- **AND** a status-zero `APICallError` with no cancellation, deadline or transport cause SHALL reduce to the status-less provider-stream error rather than the canonical upstream error
- **AND** a valid status-bearing provider error, including an SSE error reported with HTTP status 200, SHALL retain status-based classification

#### Scenario: Status-less provider error is written in place
- **WHEN** a provider emits a status-less error, as for a stream frame that failed to parse, followed by valid content and finish
- **THEN** both clients SHALL receive an error event whose base fields are exactly `{"type":"error","error":{"message":"provider stream error","type":"internal_server_error","param":null,"code":"provider_error","retryable":false}}`, with no `statusCode` member, followed by every later event in provider order

#### Scenario: Status-less provider error carries native text
- **WHEN** a status-less provider error carries a native message, native status text or provider account detail
- **THEN** none of that text SHALL appear in the error event's base fields, and optional `error.data` SHALL follow gateway-attempt-failure-evidence exactly as for any other provider error

### Requirement: Synthetic terminal adapter errors

Unsupported families, lifecycle violations, invalid output, premature close, mapping or framing failure, timeout and cancellation SHALL retain existing bounded terminal error behavior, before and after a written finish. A provider close SHALL be premature unless it ends cleanly under "Stream completion on provider close". Ordinary generated files, approvals and unsupported tool behavior SHALL remain unsupported.

#### Scenario: Reasoning lifecycle violation
- **WHEN** reasoning ends without a matching active start, reuses an ended reasoning ID within its segment, or remains open at finish
- **THEN** the handler SHALL produce at most one safe terminal error and no synthetic finish

#### Scenario: Unsupported stream family appears
- **WHEN** the text runtime receives provider-executed/dynamic/preliminary tool behavior, ordinary generated file, approval, or another unsupported part
- **THEN** it SHALL emit at most one terminal internal error rather than serializing the provider-domain part

### Requirement: Stream completion on provider close
The response SHALL end at provider close, at a terminal adapter or runtime failure, at idle timeout, request deadline or cancellation, or at writer failure, not at a finish. Lifecycle parts SHALL be every part except raw parts and provider error parts. Provider close SHALL be a clean EOF without `[DONE]` only when the last lifecycle part is a valid finish or a provider error part follows the last lifecycle part; otherwise the handler SHALL attempt one synthetic premature-close internal error.

#### Scenario: Close after a finish
- **WHEN** the provider closes its stream after a valid finish that is the last lifecycle part
- **THEN** the response SHALL end at clean EOF without `[DONE]` or another event

#### Scenario: Close without a finish
- **WHEN** the provider closes its stream before any valid finish and no provider error part follows the last lifecycle part
- **THEN** the handler SHALL attempt one synthetic internal error and SHALL NOT synthesize a finish or block end

#### Scenario: Close after a new segment opens
- **WHEN** the provider closes its stream after a valid finish followed by response-metadata, and no provider error part follows the metadata
- **THEN** the handler SHALL attempt one synthetic internal error after the metadata and SHALL NOT synthesize a finish

### Requirement: Timeout and cancellation after a finish
Idle timeout, request deadline expiry and request cancellation SHALL keep their normal terminal behavior after a written finish; only provider close SHALL depend on the last lifecycle part or a trailing provider error part. The handler SHALL cancel provider work before encoding the terminal event and write it after the finish: a timeout error for idle timeout or deadline expiry, or a cancellation error only while the writer is still usable. The finish SHALL NOT be retracted.

#### Scenario: Idle after a finish
- **WHEN** a provider writes a valid finish and then neither emits nor closes until the idle timeout
- **THEN** provider work SHALL be canceled before one timeout error event is written after the finish, the response SHALL end within the idle duration plus bounded handler latency, and cleanup SHALL remain single-owner and bounded

#### Scenario: Stall between segments
- **WHEN** a provider writes a valid finish for one message, has further messages pending and then stalls until the idle timeout or the request deadline
- **THEN** the finish SHALL remain written and one timeout error event SHALL follow it, so the response SHALL NOT end at clean EOF

#### Scenario: Cancellation after a finish
- **WHEN** the request is canceled after a valid finish that is the last lifecycle part
- **THEN** provider work SHALL be canceled and at most one cancellation error event SHALL be attempted after the finish, only if the writer is still usable

#### Scenario: Idle after a segment opens
- **WHEN** a valid finish is followed by response-metadata and the idle timeout then expires
- **THEN** provider work SHALL be canceled before one timeout error event is written

## ADDED Requirements

### Requirement: Clean end after a provider error

When the provider closes its stream and a provider error part follows the last lifecycle part, the runtime SHALL report the closed-after-error outcome and the handler SHALL end the response at clean EOF, whether or not a finish was written earlier. It SHALL NOT write a synthetic terminal error, finish or block closure. Raw parts are not lifecycle parts, so they SHALL NOT change this outcome, whether or not they are written. This rule SHALL apply to a provider error of any safe category.

#### Scenario: Provider closes right after an error
- **WHEN** a provider emits stream-start, opens a tool-input or text block, emits an error part and then closes its stream without finish
- **THEN** the provider error SHALL be the last event, the response SHALL end at clean EOF, and no `internal_error` event, block closure or finish SHALL be written

#### Scenario: Error is the first part and the stream closes
- **WHEN** the first provider part is an error and the provider then closes its stream
- **THEN** the response SHALL contain the normalized stream-start and that error, then end at clean EOF

#### Scenario: Lifecycle part follows the error before close
- **WHEN** a provider emits an error part, then a lifecycle part that is not a valid finish, and then closes
- **THEN** the handler SHALL write exactly one synthetic terminal internal error after the later event, as for any other premature close

#### Scenario: Raw part follows the error
- **WHEN** a provider emits an error part, then a raw part, and then closes
- **THEN** the response SHALL end at clean EOF, with the raw event after the error when raw output was requested and its value is usable, and with the error as the last event otherwise

#### Scenario: Close without a trailing provider error
- **WHEN** the provider closes, the last lifecycle part is not a valid finish, and no provider error part follows it, including a close before the first part
- **THEN** the existing single synthetic terminal internal error SHALL be written

#### Scenario: Other terminal outcomes after an error
- **WHEN** request cancellation, the request deadline or the idle timeout ends the stream after a provider error part
- **THEN** the handler SHALL keep the existing cancellation or timeout terminal behavior
- **AND** when such an outcome becomes ready concurrently with the provider close, either outcome MAY win, subject to the existing "Streaming terminal authority and scheduler race boundary" requirement
