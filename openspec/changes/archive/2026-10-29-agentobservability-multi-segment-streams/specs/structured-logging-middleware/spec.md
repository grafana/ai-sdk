## MODIFIED Requirements

### Requirement: Stream call logging

`WrapStream` SHALL log `EventStreamStart` before invoking `p.DoStream(ctx)` exactly once. Opening errors SHALL log `EventStreamError` with duration and sanitized details and return the original error. Success SHALL return a new `*provider.StreamResult` preserving upstream `Request`/`Response`, with a tee forwarding every part unchanged in order. The channel SHALL close exactly once and use a bounded 64-entry buffer unless a future measured change updates this requirement.

#### Scenario: Stream usage preserves strongest values

- **GIVEN** usage is split across multiple stream parts
- **AND** the finish part omits some normalized counters
- **WHEN** the stream completes
- **THEN** the terminal log record SHALL contain the counters the finish reports and, for the counters it omits, the strongest values observed before it, under shared streaming usage aggregation

#### Scenario: Stream usage follows the last finish

- **GIVEN** a stream emits several finish parts, each reporting its own usage
- **WHEN** the stream completes
- **THEN** the terminal log record SHALL contain the last finish's segment usage, not a sum or the greatest value across segments

#### Scenario: Stream success tees unmodified parts

- **GIVEN** an upstream stream emits multiple `provider.StreamPart` values
- **WHEN** a consumer reads from the logger-wrapped stream
- **THEN** the consumer SHALL receive the same parts in the same order
- **AND** the terminal log record SHALL include counts derived from those parts

#### Scenario: Stream open error logs and propagates

- **WHEN** the inner model's `DoStream` returns an error before a stream is opened
- **THEN** the wrapped call SHALL return that original error unchanged
- **AND** one `EventStreamError` record SHALL be emitted after the start record

#### Scenario: Stream part error logs terminal error

- **WHEN** the upstream stream emits a `provider.PartError` containing an `APICallError`
- **THEN** the part SHALL still be forwarded to the consumer unchanged
- **AND** the terminal record SHALL be `EventStreamError` with `ai_sdk.outcome="error"` and `ai_sdk.success=false`
- **AND** the record SHALL include sanitized API-call status/retryability details when available

#### Scenario: Stream context cancellation finalizes once

- **WHEN** the request context is cancelled while the stream tee is active
- **THEN** the tee SHALL stop blocking on downstream sends
- **AND** it SHALL close the returned stream channel exactly once
- **AND** it SHALL emit exactly one terminal `EventStreamCancelled` record for the call with `ai_sdk.outcome="cancelled"`

#### Scenario: Per-part logging is opt-in

- **WHEN** `Options.LogStreamParts` is false
- **THEN** no `EventStreamPart` records SHALL be emitted
- **WHEN** `Options.LogStreamParts` is true
- **THEN** the logger MAY emit one `EventStreamPart` record per observed stream part subject to capture and redaction policy

#### Scenario: Stream observations feed terminal summaries
- **WHEN** the tee observes stream parts
- **THEN** it SHALL accumulate counts, time to first content, response metadata from `provider.PartResponseMeta`, normalized usage from every usage-bearing part using shared aggregation, finish reason/provider metadata from `provider.PartFinish`, and the first `provider.PartError`

#### Scenario: Per-part level applies except to errors
- **WHEN** `Options.LogStreamParts` is true
- **THEN** `EventStreamPart` records SHALL use `Options.PartLevel` except for error parts
- **AND** part records SHALL be logged only when `Options.LogStreamParts` is true

#### Scenario: Upstream close selects one terminal event
- **WHEN** the upstream stream closes
- **THEN** exactly one terminal event SHALL be logged: `EventStreamFinish` for success, `EventStreamError` for a part error or deadline timeout, or `EventStreamCancelled` for cancellation
