# Spec Delta

## ADDED Requirements

### Requirement: Fixed extension error documents
Extension outcomes SHALL use fixed ProviderWire documents with `param: null`, chosen from the category alone. A denial SHALL reuse the existing permission document byte for byte. The unavailable category SHALL use HTTP 503, `internal_server_error`, code `unavailable`, message "service unavailable", retryable. Neither SHALL echo extension names, error text or configuration.

#### Scenario: Unary and setup documents
- **WHEN** a unary call or stream setup fails with an extension denial or unavailable outcome
- **THEN** the response SHALL be the fixed permission or unavailable document, with execution metadata only when an overview is published

#### Scenario: In-band stream outcome
- **WHEN** a committed stream ends with an extension denial or unavailable error part
- **THEN** the stream SHALL carry one error event with the same fixed type, code and message as the corresponding document and no native failure summary, and then end

#### Scenario: Client classification of extension documents
- **WHEN** the pinned TypeScript client and the Go client receive the denial and unavailable documents
- **THEN** they SHALL classify the denial as a forbidden error that is not retryable and the unavailable document as an internal server error that is retryable
