## MODIFIED Requirements

### Requirement: Closed failure classification
The runtime SHALL classify every failure into one closed category: invalid request, BYOK credentials, BYOK selector, unsupported gateway control, catalog configuration, model not found, permission, rate limit, overload, failed dependency, upstream, provider stream, timeout, cancellation or internal. Invalid request MAY carry a closed reason: reserved provider options, protected call header, routing controls, no candidates or attempt bound. Model not found MAY carry the fallback-model reason.

#### Scenario: Selection errors
- **WHEN** selection fails with account access denied, invalid BYOK credentials, an invalid BYOK selector, an unsupported gateway control, reserved provider options or an unknown model
- **THEN** the category SHALL be permission, BYOK credentials, BYOK selector, unsupported gateway control, invalid request with the reserved reason, or model not found respectively, and any other error SHALL be classified as a provider error

#### Scenario: Stack-catalog configuration error
- **WHEN** selection fails because a stored stack catalog fails its checks or a catalog provider has no decrypted key
- **THEN** the category SHALL be catalog configuration with no reason, before the provider-error fallback

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
