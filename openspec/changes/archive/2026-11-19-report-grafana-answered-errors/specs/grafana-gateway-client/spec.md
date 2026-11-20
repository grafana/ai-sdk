## MODIFIED Requirements

### Requirement: Error fallback messages and standard Go member matching

Except for Grafana-answered errors, unknown or malformed error bodies, wrong media types and transport failures SHALL use local bounded text, not response bytes, as the primary message. Context cancellation and deadlines SHALL remain discoverable with `errors.Is`. HTTP error envelopes and committed error payloads SHALL use standard Go JSON struct-member matching, including case-insensitive matches and standard duplicate-member processing, rather than a custom exact-name filter.

#### Scenario: Error fallback messages and standard Go member matching
- **WHEN** a valid envelope uses case-variant members and a malformed envelope contains arbitrary response bytes
- **THEN** valid typed decoding SHALL follow standard Go matching/duplicate processing, while malformed input SHALL use bounded local primary text and preserve context error identity

## ADDED Requirements

### Requirement: Grafana-answered errors

A well-formed JSON non-2xx body without an `error` member SHALL be reported as a Grafana response rather than a Gateway envelope. The client SHALL return a non-retryable `*provider.APICallError` with the HTTP status, local primary text that names Grafana followed by the body's top-level `message` string when it is valid non-blank UTF-8, and the complete bounded body in ResponseBody.

#### Scenario: Grafana answers before the Gateway
- **WHEN** a non-2xx model or discovery response is a JSON object without an `error` member, such as a Kubernetes Status document or a Grafana API error
- **THEN** the client SHALL return a non-retryable `*provider.APICallError` that is not a `GatewayError`, with the HTTP status, a primary message naming Grafana and including Grafana's `message` when present, and the complete body in ResponseBody
- **AND** a body whose `error` member is present but null or invalid SHALL keep the bounded local Gateway envelope error with an empty ResponseBody
