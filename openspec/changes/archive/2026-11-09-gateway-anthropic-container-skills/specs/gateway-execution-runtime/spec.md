# Spec Delta

## ADDED Requirements

### Requirement: Aggregate chain-failure classification
The runtime SHALL classify a chain failure that joins two or more attempt failures, or attempt failures with the call-level context error, as one failure: a call-level cancellation or expired deadline SHALL win; otherwise the newest attempt failure that is not a refusal SHALL decide by the single-error rules; only when every attempt refused SHALL it be invalid request. A refusal is an attempt failure that is invalid request through `ErrUnsupportedRequest` or `ErrUnsupportedFunctionality`.

#### Scenario: Refusal next to a provider failure
- **WHEN** a chain aggregate joins a refusal (`catalog.ErrUnsupportedRequest` or `provider.ErrUnsupportedFunctionality`) and a provider failure, in either order, such as a refusing primary and a fallback that returns 503, a 503 primary and a refusing fallback, or a refusing primary and a fallback that returns 429, and the aggregate carries no call-level cancellation or expired deadline
- **THEN** the category SHALL be that of the provider failure (overload, overload and rate limit respectively), not invalid request

#### Scenario: Most recent non-refusal failure decides
- **WHEN** a chain aggregate joins several provider failures and refusals, or joins a refusal with an error that has no other category, and the aggregate carries no call-level cancellation or expired deadline
- **THEN** the most recent attempt failure that is not a refusal SHALL decide the category through the single-error rules, so a refusal next to an unknown error, in either order, SHALL be internal
- **AND** an earlier attempt-level timeout SHALL NOT outrank a newer 503

#### Scenario: Call canceled after a refusal
- **WHEN** a chain aggregate joins one or more attempt failures, including a refusal, with the call-level cancellation or expired deadline
- **THEN** the category SHALL be cancellation or timeout respectively
- **AND** the call-level context error SHALL be the last element of the top-level join and identical to the context cancellation or deadline error; an attempt failure that merely wraps one of them SHALL NOT count as call-level

#### Scenario: Every attempt refused
- **WHEN** every attempt in a chain aggregate is a refusal, through either error, and the aggregate carries no call-level cancellation or expired deadline
- **THEN** the category SHALL be invalid request

#### Scenario: Single refusal
- **WHEN** a failure is a single refusal, not joined with another attempt failure or a call-level context error
- **THEN** the category SHALL remain invalid request

#### Scenario: Every Gateway chain
- **WHEN** configured fallback, routed or BYOK chains of any provider fail with an aggregate
- **THEN** the aggregate rule SHALL apply to each of them, through every adapter that classifies provider errors

#### Scenario: API error wrapping the unsupported-functionality error
- **WHEN** an attempt failure is an API call error or transport error that wraps the unsupported-functionality error
- **THEN** it SHALL NOT count as a refusal and SHALL keep its API call or transport category
