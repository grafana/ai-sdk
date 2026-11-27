## MODIFIED Requirements

### Requirement: Stream call instrumentation and tee behavior

`WrapStream` SHALL increment in-flight before inner `DoStream`. Opening errors SHALL record failure like generate errors, decrement in-flight, and return unchanged. A result SHALL be wrapped in a new `provider.StreamResult` preserving request/response metadata, with a tee forwarding every part unchanged in order, observing metrics, and closing downstream once. Request/duration/usage/TTFT/in-flight SHALL finalize on upstream close or context cancellation.

#### Scenario: Stream success forwards exact parts

- **WHEN** the inner model returns a stream that emits N parts and closes normally
- **THEN** the consumer SHALL receive the same N `provider.StreamPart` values in the same order
- **AND** the middleware SHALL record exactly one successful stream request when the upstream stream closes

#### Scenario: Initial stream error is returned unchanged

- **WHEN** the inner model's `DoStream` returns a non-nil error before returning a stream
- **THEN** the middleware SHALL record exactly one failed stream request
- **AND** it SHALL return the same error to the caller

#### Scenario: Stream part error records failed request

- **WHEN** the upstream stream emits a `provider.StreamPart{Type: provider.PartError}` and then closes
- **THEN** the error part SHALL be forwarded to the consumer unchanged
- **AND** the final request metrics SHALL use `status="error"`

#### Scenario: Stream in-flight gauge is balanced

- **WHEN** the stream closes or the request context is canceled
- **THEN** `aisdk_model_inflight_requests` SHALL be decremented for the same requested provider/model label set that was incremented before the inner call

#### Scenario: Cancellation stops blocked downstream sends
- **WHEN** context cancellation prevents downstream sends
- **THEN** the tee SHALL stop sending and continue draining upstream on a best-effort basis

#### Scenario: Stream error classification respects earlier cancellation
- **WHEN** an upstream `provider.PartError` is observed
- **THEN** final request status SHALL be `error` unless the context was canceled before the error part was observed, including when the context is canceled afterwards
- **AND** the error part SHALL still be forwarded unchanged

#### Scenario: Denial outranks a later cancellation
- **WHEN** the denial classifier reports an observed error part as a denial and the request context is canceled afterwards
- **THEN** final request status SHALL be `denied`
