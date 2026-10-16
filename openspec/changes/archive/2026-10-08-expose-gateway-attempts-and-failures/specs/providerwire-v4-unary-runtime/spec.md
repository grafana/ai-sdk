## MODIFIED Requirements

### Requirement: Fixed privacy-safe errors

Every runtime error response SHALL retain fixed classified status, message, type, code, and `param: null` fields from protocol-owned definitions.

#### Scenario: Fixed privacy-safe errors policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Typed standard JSON encoding SHALL preserve the canonical base response bytes. Invalid request, model-not-found, rate-limit, overload, failed-dependency, upstream, timeout, cancellation, and internal categories SHALL use Gateway-recognized error types and status-derived retryability. Unknown or invalid internal categories SHALL fall back to the fixed internal-error document.

#### Scenario: Provider API failure
- **WHEN** `DoGenerate` returns an API, transport, timeout, cancellation, or arbitrary internal error
- **THEN** the handler SHALL retain the corresponding fixed safe fields without serializing arbitrary causes; optional execution attribution SHALL follow gateway-attempt-failure-evidence

#### Scenario: Unknown model
- **WHEN** catalog resolution reports an unknown public model
- **THEN** the handler SHALL return the fixed model-not-found document

#### Scenario: Client classification
- **WHEN** the registered Gateway client consumes a fixed error document
- **THEN** its status, type, and retryability SHALL map to the expected client class

### Requirement: Runtime error detail exclusions

Arbitrary cause trees and raw bodies/headers SHALL NOT be serialized by failure projection. Optional native candidate-local summaries and observed configured identity MAY accompany the base error under gateway-attempt-failure-evidence, within existing complete-response limits; they SHALL NOT change classification or retryability. Summary protection SHALL follow gateway-attempt-failure-evidence.

#### Scenario: Runtime error detail exclusions
- **WHEN** a resolver or provider failure carries URL, credentials and metadata
- **THEN** arbitrary cause trees and raw bodies/headers SHALL NOT be serialized; governed native candidate-local summaries SHALL redact exact configured API-key matches while preserving other scalar echoes and fixed base fields

### Requirement: Host-safe ProviderWire error writer

The `ai-gateway/providerwire/v4` package SHALL expose a narrow host-composition error writer for failures outside the language-model handler, including service authentication, permission, and discovery failures. Construction SHALL be non-fallible and SHALL accept no configuration.

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

### Requirement: Optional unary execution overview

The handler SHALL use synchronized request-local observation to optionally enrich a valid unary result or classified invocation failure with a compact execution overview.

#### Scenario: Optional unary execution overview policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Original primary validation and existing complete-response limits SHALL apply before accepting enrichment. Optional encoding/fit failure SHALL preserve the original document and classification; original adaptation failures SHALL NOT replay generation. Configured candidate identity SHALL remain separate from native-summary eligibility; only explicit configured API keys SHALL supply summary protection; request credentials SHALL NOT be inventoried. Configured direct calls SHALL retain their existing invocation/cancellation ownership.

#### Scenario: Valid result with no enrichment room
- **WHEN** a native result fits but adding or relocating the overview exceeds the existing response bound
- **THEN** the original successful result SHALL be written without metadata loss or a diagnostic-induced failure

#### Scenario: Owned cancellation and late native result
- **WHEN** cancellation wins invocation ownership before a late native result
- **THEN** the existing context error SHALL be returned and published attribution SHALL NOT capture the unowned late native failure
