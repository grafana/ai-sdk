## MODIFIED Requirements

### Requirement: Closed Gateway error classification
Every non-2xx model or discovery response SHALL be read within the configured error-body limit and mapped only from the registered public error envelope into the closed categories authentication, forbidden, invalid request, model not found, rate limit, failed dependency, and internal server. The resulting `GatewayError` SHALL expose category, public code, public message, HTTP status, and status-derived retryability and SHALL unwrap to a bounded `*provider.APICallError`. For a valid registered envelope it SHALL retain the complete bounded JSON document in that cause's Data and ResponseBody, including additive providerMetadata.gateway.evidence, rather than reconstructing only the registered error fields. Decoding SHALL remain independent of Gateway imports and SHALL NOT introduce a new public error API or interpret every opaque evidence member. Unknown or malformed error bodies, wrong media types, and transport failures SHALL use local bounded error text rather than copying arbitrary response bytes into the primary message. Context cancellation and deadlines SHALL remain discoverable with `errors.Is`. HTTP error envelopes and committed error payloads SHALL use standard Go JSON struct-member matching, including case-insensitive matches and standard duplicate-member processing, rather than a custom exact-name filter. Opaque raw data SHALL remain unmodified by that typed decoding.

#### Scenario: Registered error is returned
- **WHEN** the server returns one of the registered error type/status/code documents
- **THEN** `errors.As` SHALL find the matching `GatewayError` category and underlying `*provider.APICallError`, and retryability SHALL match the Gateway version registered in test/conformance/upstream.yaml, independently of nested native retry facts

#### Scenario: Error members use noncanonical property casing
- **WHEN** an otherwise valid HTTP error envelope or committed error payload uses case-insensitive Go matches for its member names
- **THEN** typed error decoding SHALL accept those members using standard encoding/json behavior
- **AND** retained HTTP documents and selected SSE data SHALL preserve their original JSON bytes without changing classification or retryability rules

#### Scenario: Error body is malformed or oversized
- **WHEN** a non-2xx body is malformed, has a wrong media type, or exceeds the error limit
- **THEN** the client SHALL return a bounded local `*provider.APICallError` without accepting a partial category or exposing the full body in its message

#### Scenario: Transport fails
- **WHEN** HTTP transport fails without context cancellation
- **THEN** the client SHALL return a retryable `*provider.APICallError` that retains the transport cause

#### Scenario: Stream error event is received
- **WHEN** a valid registered error event appears in SSE
- **THEN** it SHALL become an ordered `PartError` with the equivalent bounded `APICallError`, preserving the Go provider contract without ending later valid parts by itself
- **AND** supplied bounded valid error.data SHALL be retained exactly in APICallError.Data, independently of mapped status/retryability, without manufacturing data when absent
- **AND** APICallError.ResponseBody SHALL remain empty rather than manufacturing an HTTP response document from the SSE event

#### Scenario: All-failed data survives classification
- **WHEN** a valid registered non-2xx envelope contains bounded attributed attempts and heterogeneous native errors
- **THEN** errors.As SHALL expose the existing GatewayError and API-call cause with complete envelope Data, while outer status/code/category and hop retryability remain unchanged

#### Scenario: Consumer middleware inspects stream data
- **WHEN** a consumer wrapper reads an attributed PartError followed by valid text and finish
- **THEN** it SHALL observe the retained data and original event order without enabling operator capture or changing core result/error policy
