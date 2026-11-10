## ADDED Requirements

### Requirement: Fixed egress refused error document
The egress refused category SHALL use one fixed ProviderWire document: HTTP 424, type `failed_dependency`, code `failed_dependency`, `param: null`, message "provider endpoint refused by the egress policy or not resolvable", not retryable. It SHALL NOT carry the endpoint, a resolved or resolver address, a range, a list entry, keys or execution metadata.

#### Scenario: Unary and streaming calls to a refused endpoint
- **WHEN** a unary call or a stream setup fails with the egress refused category
- **THEN** the response SHALL be that fixed document before any stream event, and the endpoint SHALL receive no request

#### Scenario: Client classification
- **WHEN** the pinned TypeScript client and the Go client receive the document
- **THEN** each SHALL classify it with its existing failed-dependency handling, as not retryable, and the Go client SHALL NOT report a protocol error
