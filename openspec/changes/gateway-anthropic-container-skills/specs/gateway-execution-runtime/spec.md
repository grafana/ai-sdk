# Spec Delta

## MODIFIED Requirements

### Requirement: Closed failure classification
The runtime SHALL classify every failure into one closed category: invalid request, BYOK credentials, BYOK selector, unsupported gateway control, model not found, permission, rate limit, overload, failed dependency, upstream, provider stream, timeout, cancellation or internal. Invalid request MAY carry a closed reason: reserved provider options, protected call header, routing controls, no candidates or attempt bound. Model not found MAY carry the fallback-model reason. A refusal is an attempt failure that the single-error rules classify as invalid request through the unsupported-request error (`catalog.ErrUnsupportedRequest`) or the unsupported-functionality error (`provider.ErrUnsupportedFunctionality`); an API call error or transport error that wraps the unsupported-functionality error stays an API call error or transport error. An aggregate is a chain failure that joins two or more attempt failures, or one or more attempt failures with the call-level context error; the call-level context error is the last element of the top-level join when it is identical to the context cancellation or deadline error, and an attempt failure that merely wraps one of them is not call-level. An aggregate SHALL be classified as one failure: a call-level cancellation or expired deadline SHALL win first; otherwise the most recent attempt failure that is not a refusal SHALL decide through the single-error rules; the aggregate SHALL be invalid request only when every attempt refused. A failure that is not an aggregate SHALL keep the single-error rules.

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

#### Scenario: Refusal next to a provider failure
- **WHEN** a chain aggregate joins a refusal (unsupported request or unsupported functionality) and a provider failure, in either order, such as a refusing primary and a fallback that returns 503, a 503 primary and a refusing fallback, or a refusing primary and a fallback that returns 429, and the aggregate carries no call-level cancellation or expired deadline
- **THEN** the category SHALL be that of the provider failure (overload, overload and rate limit respectively), not invalid request

#### Scenario: Most recent non-refusal failure decides
- **WHEN** a chain aggregate joins several provider failures and refusals, or joins a refusal with an error that has no other category, and the aggregate carries no call-level cancellation or expired deadline
- **THEN** the most recent attempt failure that is not a refusal SHALL decide the category through the single-error rules, so a refusal next to an unknown error, in either order, SHALL be internal
- **AND** an earlier attempt-level timeout SHALL NOT outrank a newer 503

#### Scenario: Call canceled after a refusal
- **WHEN** a chain aggregate joins one or more attempt failures, including a refusal, with the call-level cancellation or expired deadline
- **THEN** the category SHALL be cancellation or timeout respectively
- **AND** an attempt failure that merely wraps a cancellation or deadline error SHALL NOT count as call-level

#### Scenario: Every attempt refused
- **WHEN** every attempt in a chain aggregate is a refusal, through either error, and the aggregate carries no call-level cancellation or expired deadline
- **THEN** the category SHALL be invalid request

#### Scenario: Single refusal
- **WHEN** a failure is a single refusal, not joined with another attempt failure or a call-level context error
- **THEN** the category SHALL remain invalid request

#### Scenario: Every Gateway chain
- **WHEN** configured fallback, routed or BYOK chains of any provider fail with an aggregate
- **THEN** the aggregate rule SHALL apply to each of them, through every adapter that classifies provider errors
