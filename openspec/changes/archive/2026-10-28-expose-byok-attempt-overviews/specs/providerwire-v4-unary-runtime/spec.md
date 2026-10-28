# Spec Delta

## MODIFIED Requirements

### Requirement: Fixed privacy-safe errors
Every runtime error response SHALL retain fixed classified status, message, type, code, and `param: null` fields from protocol-owned definitions.

#### Scenario: Fixed privacy-safe errors policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Typed standard JSON encoding SHALL preserve the canonical base response bytes. Invalid request, model-not-found, rate-limit, overload, failed-dependency, upstream, timeout, cancellation, and internal categories SHALL use Gateway-recognized error types and status-derived retryability. Unknown or invalid internal categories SHALL fall back to the fixed internal-error document.

#### Scenario: Authenticated fixed error policy
- **WHEN** configured or BYOK invocation produces a classified failure
- **THEN** Every runtime error response SHALL retain fixed classified status, message, type, code, and `param: null` fields from protocol-owned definitions.
- **AND** Typed standard JSON encoding SHALL preserve the canonical base response bytes.
- **AND** Invalid request, model-not-found, rate-limit, overload, failed-dependency, upstream, timeout, cancellation, and internal categories SHALL use Gateway-recognized error types and status-derived retryability.
- **AND** Unknown or invalid internal categories SHALL fall back to the fixed internal-error document.
- **AND** Arbitrary aggregate/configuration/cause trees and raw bodies/headers SHALL NOT be attached as diagnostic shortcuts. Gateway-produced native summary fields SHALL protect exact configured provider/exporter keys under gateway-attempt-failure-evidence, preserving unrelated BYOK echoes and source data.
- **AND** Optional native candidate-local summaries and observed configured or request-account identity MAY accompany the base error under gateway-attempt-failure-evidence, encoded once with the base error; they SHALL NOT change classification or retryability.
- **AND** Host-owned BYOK validation and unsupported-operation failures SHALL use fixed approved messages without copying rejected values or private diagnostics.
- **AND** Stopped or exhausted credential fallback SHALL retain existing fallback error propagation and safe runtime classification rather than use a blanket exhaustion status.
- **AND** Request-only selections SHALL remain eligible for available event-local SSE native summaries, and their unary/setup failures SHALL carry the request-account overview with candidate-local native summaries when at least one account was invoked.

#### Scenario: Actionable credential shape failure
- **WHEN** a credential entry is missing apiKey or the selected provider's array exceeds the account count
- **THEN** the invalid-request response SHALL identify the known field/capability without copying the entry or value

#### Scenario: Discovery unsupported for BYOK
- **WHEN** authenticated BYOK access requests catalog discovery
- **THEN** the host SHALL emit HTTP 400, invalid_request_error, invalid_request, param null and a fixed unsupported-discovery message

#### Scenario: Provider API failure
- **WHEN** `DoGenerate` returns an API, transport, timeout, cancellation, or arbitrary internal error
- **THEN** the handler SHALL retain the corresponding fixed safe fields without serializing arbitrary causes; optional native configured or request-account execution attribution SHALL follow gateway-attempt-failure-evidence

#### Scenario: Unknown model
- **WHEN** configured catalog resolution reports an unknown public model
- **THEN** the handler SHALL return the fixed model-not-found document

#### Scenario: Client classification
- **WHEN** Go and pinned Vercel consume BYOK validation, exhaustion or discovery failures
- **THEN** status/type/retryability SHALL match the existing client categories without misclassifying provider rejection as failed Gateway authentication

#### Scenario: BYOK provider echoes request accounts
- **WHEN** authenticated request-only selection receives a native failure containing request-account keys or identifiers before or after stream commitment
- **THEN** fixed classified error fields SHALL remain unchanged, and unary/setup overview summaries and SSE native summaries SHALL preserve unrelated BYOK echoes and protect exact configured provider/exporter-key matches independently of catalog attribution
