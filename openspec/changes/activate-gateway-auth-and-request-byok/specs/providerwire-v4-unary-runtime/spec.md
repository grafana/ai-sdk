## MODIFIED Requirements

### Requirement: Fixed privacy-safe errors
Every runtime error response SHALL retain fixed classified status, message, type, code, and `param: null` fields from protocol-owned definitions. Typed standard JSON encoding SHALL preserve the canonical base response bytes. Invalid request, model-not-found, rate-limit, overload, failed-dependency, upstream, timeout, cancellation, and internal categories SHALL use Gateway-recognized error types and status-derived retryability. Unknown or invalid internal categories SHALL fall back to the fixed internal-error document. Arbitrary provider, transport, resolver and panic causes, raw bodies/headers and credentials SHALL NOT be serialized. Optional protected candidate-local summaries and observed configured identity MAY accompany the base error under gateway-attempt-failure-evidence, within complete-response limits; they SHALL NOT change classification or retryability.

Host-owned BYOK validation and unsupported-operation failures SHALL use fixed approved messages without copying rejected values or private diagnostics. Stopped or exhausted credential fallback SHALL retain existing fallback error propagation and safe runtime classification rather than use a blanket exhaustion status. Request-only selection SHALL NOT activate configured public execution overviews or native failure summaries.

#### Scenario: Actionable credential shape failure
- **WHEN** a credential entry is missing apiKey or a supplied provider array exceeds the account count
- **THEN** the invalid-request response SHALL identify the known field/capability without copying the entry or value

#### Scenario: Discovery unsupported for BYOK
- **WHEN** authenticated BYOK access requests catalog discovery
- **THEN** the host SHALL emit HTTP 400, invalid_request_error, invalid_request, param null and a fixed unsupported-discovery message

#### Scenario: Provider API failure
- **WHEN** `DoGenerate` returns an API, transport, timeout, cancellation, or arbitrary internal error
- **THEN** the handler SHALL retain the corresponding fixed safe fields without serializing arbitrary causes; optional protected configured execution attribution SHALL follow gateway-attempt-failure-evidence

#### Scenario: Unknown model
- **WHEN** configured catalog resolution reports an unknown public model
- **THEN** the handler SHALL return the fixed model-not-found document

#### Scenario: Client classification
- **WHEN** Go and pinned Vercel consume BYOK validation, exhaustion or discovery failures
- **THEN** status/type/retryability SHALL match the existing client categories without misclassifying provider rejection as failed Gateway authentication

#### Scenario: BYOK provider echoes request accounts
- **WHEN** authenticated request-only selection receives a native failure containing request-account keys or identifiers before or after stream commitment
- **THEN** fixed classified error fields SHALL remain unchanged and the Gateway SHALL omit execution overviews and current native failure summaries without exposing the echoed values

### Requirement: Host-safe ProviderWire error writer
The `ai-gateway/providerwire/v4` package SHALL expose a narrow host-composition error writer for failures outside the language-model handler, including service authentication, permission, and discovery failures. Construction SHALL be non-fallible and SHALL accept no configuration. Callers SHALL select only closed authentication, permission, internal or unsupported-BYOK-discovery categories; they SHALL NOT supply public messages, causes, arbitrary status codes, error types, error codes, retryability, or byte limits.

The writer SHALL use the same protocol-owned public error definitions and typed HTTP encoder as the language-model handler, without optional execution metadata. It SHALL perform no runtime schema compilation or validation; encoding SHALL use only fixed package-owned values. Authentication SHALL emit the package's exact fixed 401 document, permission SHALL emit the exact fixed 403 document, internal SHALL emit the exact fixed 500 document, and an invalid category SHALL select that same internal document. The API SHALL not expose private DTOs or weaken the strict handler's existing failure bytes.

#### Scenario: Host emits authentication failure
- **WHEN** authenticated service middleware selects the closed authentication category
- **THEN** the writer SHALL emit the exact fixed 401 authentication document owned by the protocol package
- **AND** no host or verifier error text SHALL be accepted or copied

#### Scenario: Host emits permission failure
- **WHEN** host authorization selects the closed permission category
- **THEN** the writer SHALL emit the exact fixed 403 forbidden document owned by the protocol package
- **AND** no caller-controlled field SHALL affect the document

#### Scenario: Host emits internal discovery failure
- **WHEN** bounded discovery encoding fails and the host selects the closed internal category
- **THEN** the writer SHALL emit the exact fixed internal-error document without a partial discovery response

#### Scenario: Host selects an invalid category
- **WHEN** an invalid typed category reaches the writer
- **THEN** it SHALL emit the same fixed canonical internal-error document
- **AND** it SHALL not serialize the invalid value

#### Scenario: Host refuses BYOK discovery
- **WHEN** authenticated BYOK discovery selects the closed unsupported category
- **THEN** the writer SHALL emit the fixed HTTP 400 invalid-request discovery document without catalog access or caller-supplied prose
