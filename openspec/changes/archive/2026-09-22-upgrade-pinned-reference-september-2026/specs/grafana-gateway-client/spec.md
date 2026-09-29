## MODIFIED Requirements

### Requirement: Bounded normalized unary consumption
For a successful unary response, the client SHALL require a JSON media type, read no more than the configured unary-response byte limit, accept one complete JSON document, and map only registered text content, finish reason, and usage into provider.GenerateResult. It SHALL reject malformed required fields, unknown content or finish discriminators, negative or non-JavaScript-safe known usage, trailing JSON, and oversized input. The client SHALL replace server-supplied request and response: Request.Body SHALL be the locally encoded request, and Response.Headers and Response.Body SHALL come from the bounded HTTP response. Warnings SHALL preserve valid server warning fields in order, defaulting to a non-nil empty slice when absent or null. Warning decoding SHALL use the same registered types and validation as streaming. Server response identity and provider-private metadata SHALL not be adopted.

#### Scenario: Minimal unary success is consumed
- **WHEN** the server returns valid ordered text content, registered finish reason, and valid usage
- **THEN** the client SHALL return those fields plus local request metadata, HTTP response headers/body, and empty warnings when none were supplied

#### Scenario: Server supplies client-owned fields
- **WHEN** a successful body includes warnings, request metadata, response ID, model ID, timestamp, headers, body, or additional provider metadata
- **THEN** the result SHALL preserve valid warnings but ignore the other server-owned values in favor of the registered client-owned replacements

#### Scenario: Unary response exceeds its bound
- **WHEN** the response is one byte larger than the configured unary limit or has trailing data
- **THEN** the call SHALL fail without returning a partial result or retaining an unbounded body

#### Scenario: Unary result is not in the WP5 text family
- **WHEN** a successful body contains an output discriminator not owned by the text client
- **THEN** the client SHALL fail explicitly until the capability's later work package extends the closed mapper

#### Scenario: Unary warning is malformed
- **WHEN** a warning has an unknown discriminator or lacks a required field
- **THEN** the call SHALL fail rather than exposing unvalidated warning content
