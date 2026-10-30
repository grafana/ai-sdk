## MODIFIED Requirements

### Requirement: Closed failure classification
The runtime SHALL classify every failure into one closed category: invalid request, BYOK credentials, BYOK selector, unsupported gateway control, model not found, permission, rate limit, overload, failed dependency, upstream, provider stream, timeout, cancellation or internal. Invalid request MAY carry a closed reason: reserved provider options, protected call header, routing controls, no candidates or attempt bound. Model not found MAY carry the fallback-model reason.

#### Scenario: Selection errors
- **WHEN** selection fails with account access denied, invalid BYOK credentials, an invalid BYOK selector, an unsupported gateway control, reserved provider options or an unknown model
- **THEN** the category SHALL be permission, BYOK credentials, BYOK selector, unsupported gateway control, invalid request with the reserved reason, or model not found respectively, and any other error SHALL be classified as a provider error

#### Scenario: Routing selection errors
- **WHEN** selection fails with malformed routing controls, a filter that excludes every candidate, a composed chain above the attempt bound or an unknown fallback model
- **THEN** the category SHALL be invalid request with the routing-controls, no-candidates or attempt-bound reason respectively, or model not found with the fallback-model reason

#### Scenario: Provider errors
- **WHEN** invocation or setup fails
- **THEN** an unsupported request or unsupported functionality SHALL be invalid request, context cancellation SHALL be cancellation, an expired deadline SHALL be timeout, provider status 408 or 504 SHALL be timeout, 429 rate limit, 503 or 529 overload, another 4xx failed dependency and any other status upstream
- **AND** a transport error SHALL be timeout when it reports a timeout and upstream otherwise, and any other error SHALL be internal
- **AND** a provider error with no status that is not cancellation, timeout or a transport error SHALL otherwise remain upstream and SHALL never be provider stream

#### Scenario: In-band stream errors
- **WHEN** a stream error part carries a provider error with no status
- **THEN** it SHALL be cancellation, timeout or the transport category when it wraps the corresponding cause, and provider stream otherwise
- **AND** no other classification SHALL produce provider stream, and no classification SHALL panic
- **AND** a status outside 100–599 SHALL be internal, and a valid status SHALL use the same status mapping as provider errors

#### Scenario: Adapter documents
- **WHEN** ProviderWire encodes a failure
- **THEN** it SHALL choose its fixed document from the category and reason alone, and every document other than the unsupported-gateway-control message, the new routing documents and the provider-stream document SHALL equal those written before this change
- **AND** provider stream SHALL use the fixed status-less stream document defined by providerwire-v4-streaming-runtime

#### Scenario: Inconsistent tool history
- **WHEN** a call fails because its prompt tool history is inconsistent
- **THEN** selection classification SHALL return invalid request with no reason, before the provider-error fallback

#### Scenario: Invalid output
- **WHEN** a stream wait or a unary call reports invalid output
- **THEN** provider classification SHALL return internal

### Requirement: Stream end decided by the runtime
The runtime SHALL decide whether a provider close ends a stream cleanly. A close after an accepted `finish`, with no lifecycle part since, SHALL return the finished outcome, a terminal success with no error that adapters SHALL NOT classify as a failure. Otherwise, a close after an accepted error part with no lifecycle part since SHALL return the closed-after-error outcome, which adapters SHALL NOT classify. Any other close SHALL return the closed outcome.

#### Scenario: Close after finish
- **WHEN** the provider closes the stream after a valid `finish`, with only raw or provider error parts after it
- **THEN** the wait SHALL return finished, and the call SHALL be reported as a success

#### Scenario: Premature close
- **WHEN** the provider closes the stream before any `finish`, or after a `response-metadata` that followed the last `finish`, with no error part after the last lifecycle part
- **THEN** the wait SHALL return closed

#### Scenario: Close after an error
- **WHEN** the provider closes the stream after an error part, with only raw or error parts after it and no `finish` as the last lifecycle part
- **THEN** the wait SHALL return closed after error with a nil error, because the error part already reported the failure, and adapters SHALL end the stream without a terminal error
- **AND** a lifecycle part after the error SHALL make a later close return closed again

#### Scenario: Stall after finish
- **WHEN** the idle duration expires, the deadline passes or the request is canceled after an accepted `finish` or error part and before the provider closes
- **THEN** the wait SHALL return idle timeout, deadline or canceled respectively, as before this change

#### Scenario: ProviderWire end of stream
- **WHEN** ProviderWire's stream ends with finished, closed after error, closed or a context outcome
- **THEN** it SHALL write nothing more after finished or closed after error, and its terminal error otherwise
