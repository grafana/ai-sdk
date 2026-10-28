## MODIFIED Requirements

### Requirement: Closed failure classification
The runtime SHALL classify every failure into one closed category: invalid request, BYOK credentials, BYOK selector, unsupported gateway control, model not found, permission, rate limit, overload, failed dependency, upstream, provider stream, timeout, cancellation or internal. Provider stream SHALL be produced only for in-band stream error parts. Invalid request MAY carry a closed reason: reserved provider options or protected call header. Classification SHALL never panic.

#### Scenario: Selection errors
- **WHEN** selection fails with account access denied, invalid BYOK credentials, an invalid BYOK selector, an unsupported gateway control, reserved provider options or an unknown model
- **THEN** the category SHALL be permission, BYOK credentials, BYOK selector, unsupported gateway control, invalid request with the reserved reason, or model not found respectively, and any other error SHALL be classified as a provider error

#### Scenario: Provider errors
- **WHEN** invocation or setup fails
- **THEN** an unsupported request or unsupported functionality SHALL be invalid request, context cancellation SHALL be cancellation, an expired deadline SHALL be timeout, provider status 408 or 504 SHALL be timeout, 429 rate limit, 503 or 529 overload, another 4xx failed dependency and any other status upstream
- **AND** a transport error SHALL be timeout when it reports a timeout and upstream otherwise, and any other error SHALL be internal
- **AND** a provider error with no status that is not cancellation, timeout or a transport error SHALL otherwise remain upstream and SHALL never be provider stream

#### Scenario: In-band stream errors
- **WHEN** a stream error part carries a provider error with no status
- **THEN** it SHALL be cancellation, timeout or the transport category when it wraps the corresponding cause, and provider stream otherwise
- **AND** a status outside 100–599 SHALL be internal, and a valid status SHALL use the same status mapping as provider errors

#### Scenario: Adapter documents
- **WHEN** ProviderWire encodes a failure
- **THEN** it SHALL choose its fixed document from the category and reason alone
- **AND** every category other than provider stream SHALL keep the document written before this change, and provider stream SHALL use the fixed status-less stream document defined by providerwire-v4-streaming-runtime
