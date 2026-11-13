# Spec Delta

## ADDED Requirements

### Requirement: Gateway extension error documents
The client SHALL accept the Gateway's extension unavailable document, HTTP 503 with type `internal_server_error` and code `unavailable`, as a retryable internal server error, and the extension denial as the existing forbidden document. Both SHALL be accepted as HTTP error documents and as SSE error events, whose retryability SHALL match those categories.

#### Scenario: Unavailable document
- **WHEN** a model call receives the unavailable document
- **THEN** `errors.As` SHALL find a `GatewayError` with the internal server category, code `unavailable`, status 503 and retryability true

#### Scenario: In-band extension events
- **WHEN** a stream carries a forbidden error event with status 403 or an unavailable error event with status 503
- **THEN** each SHALL become a `PartError` with that status and retryability, and an unavailable event that claims not to be retryable SHALL be refused as a protocol error
