# Spec Delta

## MODIFIED Requirements

### Requirement: Unsupported-functionality rejection classification
When unary invocation or stream setup fails with the provider unsupported-functionality rejection, the handler SHALL return the fixed non-retryable invalid-request error, which MAY be accompanied by the optional execution overview. Existing unsupported-request, cancellation, deadline, API-call-error and transport classification SHALL take precedence; a chain aggregate SHALL follow the gateway-execution-runtime aggregate rule.

#### Scenario: Direct rejection
- **WHEN** a direct route's adapter returns a wrapped unsupported-functionality rejection before any I/O, in unary or streaming setup
- **THEN** the handler SHALL return the fixed invalid-request document and neither registered client SHALL retry the request

#### Scenario: All candidates reject
- **WHEN** every configured-fallback candidate returns the rejection
- **THEN** the handler SHALL return the fixed invalid-request document

#### Scenario: Operational failures keep existing classification
- **WHEN** fallback attempts end with the rejection and a retryable 503, in either order
- **THEN** the handler SHALL keep the existing 503 classification

#### Scenario: Rejection next to an unknown error
- **WHEN** fallback attempts end with the rejection and an error that has no other category, in either order
- **THEN** the handler SHALL return the fixed internal-error document, because the unknown error is the most recent attempt failure that is not a refusal

#### Scenario: API call error wrapping the sentinel
- **WHEN** an API call error's cause wraps the sentinel
- **THEN** the handler SHALL keep the existing API-call-error classification

#### Scenario: Cancellation wins
- **WHEN** the request is canceled after a candidate rejected
- **THEN** the handler SHALL keep the existing cancellation classification
