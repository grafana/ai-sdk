## ADDED Requirements

### Requirement: Successful unary transport read failures retain retryability
When a successful HTTP 200 `DoGenerate` response body fails to read because of transport I/O after headers, the Grafana client SHALL return a retryable `*provider.APICallError` with HTTP status 200, a bounded locally worded primary message, and a discoverable underlying read cause, without returning a partial result or making another model request. Wrong media type, malformed JSON, strict output-schema violations, and unary byte-limit violations SHALL remain non-retryable protocol failures. Context cancellation or deadline expiry SHALL remain discoverable with `errors.Is` and SHALL NOT be classified as a retryable transport failure. Non-2xx Gateway error responses and discovery retain their existing classification; this requirement does not add any new public service-error category.

#### Scenario: HTTP 200 response body is interrupted after headers
- **WHEN** the real HTTP handler sends JSON headers and a partial, below-limit body with a declared longer `Content-Length`, then closes the connection before the body completes
- **THEN** `DoGenerate` SHALL return no result and one retryable `*provider.APICallError` with status 200, a locally bounded primary message that does not disclose the response body, and the underlying body-read failure discoverable through its cause chain, after exactly one model request

#### Scenario: Successful unary body cannot be accepted for protocol reasons
- **WHEN** a successful unary response has a wrong media type, malformed JSON, invalid registered result schema, or exceeds the configured unary byte limit, including when an over-limit partial body and a transport read error occur together
- **THEN** `DoGenerate` SHALL return no partial result and a non-retryable bounded protocol error after exactly one model request; the unary byte limit SHALL take precedence over the read error, except that context cancellation or deadline expiry SHALL retain highest precedence

#### Scenario: Unary read is canceled
- **WHEN** the call context is canceled or reaches its deadline while a successful unary response body is being read
- **THEN** `DoGenerate` SHALL preserve context error identity through `errors.Is`, SHALL NOT return a retryable transport error, and SHALL NOT replay the request

#### Scenario: Stream parts precede a transport failure
- **WHEN** an established `DoStream` response delivers one or more parts and then its transport fails
- **THEN** the client SHALL NOT issue another HTTP request or replay any previously delivered part
