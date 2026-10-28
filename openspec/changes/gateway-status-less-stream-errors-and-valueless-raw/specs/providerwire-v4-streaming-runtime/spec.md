## MODIFIED Requirements

### Requirement: Ordered non-terminal provider errors

Each pre-finish provider `PartError` SHALL be independently reduced through the closed safe-error classification and emitted in place with base fields `{"type":"error","error":{"message":string,"type":string,"param":null,"code":string,"statusCode":integer,"retryable":boolean}}`. The provider-stream category SHALL instead use the fixed base fields `{"type":"error","error":{"message":"provider stream error","type":"internal_server_error","param":null,"code":"provider_error","retryable":false}}`, with no `statusCode` member.

#### Scenario: Ordered non-terminal provider errors policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Optional `error.data` MAY carry native current-error and execution summaries under gateway-attempt-failure-evidence. A valid provider error SHALL not terminate the stream or alter lifecycle state; later metadata, content, additional provider errors, and finish SHALL remain valid.

#### Scenario: Provider error is followed by content
- **WHEN** a provider emits an error before or within a text block and later emits otherwise valid content and finish
- **THEN** the client SHALL receive the safe error and every later event in provider order

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
- **THEN** both clients SHALL receive an error event whose base fields are exactly the provider-stream document, with no `statusCode` and `retryable` false, followed by every later event in provider order

#### Scenario: Status-less provider error carries native text
- **WHEN** a status-less provider error carries a native message, native status text or provider account detail
- **THEN** none of that text SHALL appear in the error event's base fields, and optional `error.data` SHALL follow gateway-attempt-failure-evidence exactly as for any other provider error

### Requirement: Synthetic terminal adapter errors

Before valid finish, unsupported families, lifecycle violations, invalid output, premature close other than a clean end after a provider error, mapping or framing failure, timeout and cancellation SHALL retain existing bounded terminal error behavior. Reasoning start/delta/end and atomic reasoning-file events SHALL be supported under gateway-reasoning-content, URL/document sources SHALL be supported under gateway-sources, and atomic custom events SHALL be supported under gateway-custom-content. Raw parts SHALL follow gateway-raw-output. Ordinary generated files, approvals and unsupported tool behavior SHALL remain unsupported.

#### Scenario: Reasoning lifecycle violation
- **WHEN** reasoning ends without a matching active start, reuses an ended reasoning ID, or remains open at finish
- **THEN** the handler SHALL produce at most one safe terminal error and no synthetic finish

#### Scenario: Unsupported stream family appears
- **WHEN** the text runtime receives provider-executed/dynamic/preliminary tool behavior, ordinary generated file, approval, or another unsupported part
- **THEN** it SHALL emit at most one terminal internal error rather than serializing the provider-domain part

#### Scenario: Custom stream part is supported
- **WHEN** the runtime receives a well-formed custom part between stream start and finish
- **THEN** it SHALL emit the custom event and continue the stream rather than emitting a terminal error

#### Scenario: Concurrent reasoning
- **WHEN** two reasoning blocks and a text block sharing one raw ID interleave
- **THEN** family-specific active state SHALL preserve each block independently and IDs SHALL not be rewritten

## ADDED Requirements

### Requirement: Clean end after a provider error

When the provider closes its stream before a valid finish and the last public event written was a provider error event, the handler SHALL end the response at clean EOF. It SHALL NOT write a synthetic terminal error, finish or block closure. A provider part that writes no event, such as a dropped raw part, SHALL NOT change which event was written last. This rule SHALL apply to a provider error of any safe category.

#### Scenario: Provider closes right after an error
- **WHEN** a provider emits stream-start, opens a tool-input or text block, emits an error part and then closes its stream without finish
- **THEN** the provider error SHALL be the last event, the response SHALL end at clean EOF, and no `internal_error` event, block closure or finish SHALL be written

#### Scenario: Error is the first part and the stream closes
- **WHEN** the first provider part is an error and the provider then closes its stream
- **THEN** the response SHALL contain the normalized stream-start and that error, then end at clean EOF

#### Scenario: Output follows the error before close
- **WHEN** a provider emits an error part, then a part that writes an event, and then closes without finish
- **THEN** the handler SHALL write exactly one synthetic terminal internal error after the later event, as for any other premature close

#### Scenario: Dropped raw part follows the error
- **WHEN** a provider emits an error part, then a raw part that is not written because raw output was not requested or its value is unusable, and then closes
- **THEN** the response SHALL end at clean EOF after the error

#### Scenario: Close without any provider error
- **WHEN** the provider closes before finish and no provider error event was the last event written, including a close before the first part
- **THEN** the existing single synthetic terminal internal error SHALL be written

#### Scenario: Other terminal outcomes after an error
- **WHEN** request cancellation, the request deadline or the idle timeout ends the stream after a provider error event
- **THEN** the handler SHALL keep the existing cancellation or timeout terminal behavior
- **AND** when such an outcome becomes ready concurrently with the provider close, either outcome MAY win, subject to the existing "Streaming terminal authority and scheduler race boundary" requirement
