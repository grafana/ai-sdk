## ADDED Requirements

### Requirement: Fixed catalog configuration error document
The catalog configuration category SHALL use one fixed ProviderWire document: HTTP 424, type `failed_dependency`, code `failed_dependency`, `param: null`, message "model catalog is invalid or missing a provider key", not retryable. It SHALL NOT carry catalog content, check messages, names, keys or execution metadata. It is not 400, because the caller's request is valid, nor 500, because the Gateway did not fail and a retry cannot succeed until the catalog changes.

#### Scenario: Unary and streaming calls on an invalid catalog
- **WHEN** a unary call or a stream setup fails with the catalog configuration category
- **THEN** the response SHALL be that fixed document before any stream event, with zero provider requests

#### Scenario: Client classification
- **WHEN** the pinned TypeScript client and the Go client receive the document
- **THEN** each SHALL classify it with its existing failed-dependency handling, as not retryable, and the Go client SHALL NOT report a protocol error
