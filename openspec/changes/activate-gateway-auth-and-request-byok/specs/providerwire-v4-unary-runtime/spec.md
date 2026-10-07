## MODIFIED Requirements

### Requirement: Fixed privacy-safe errors
Runtime failures SHALL use the existing closed Gateway-recognized status/type/code categories with param null and status-derived retryability. Generic failures SHALL retain fixed safe documents. Host-owned BYOK validation and unsupported-operation failures SHALL use bounded approved message templates naming only known schema paths/capabilities and bounded indices/counts. Stopped or exhausted credential fallback SHALL retain existing fallback error propagation and safe runtime classification rather than use a blanket exhaustion status. Any selected-provider/attempt-count diagnostics SHALL remain bounded and credential-free. Unknown categories SHALL become fixed internal errors.

No rejected values, arbitrary member names, raw bodies, authentication material or unrestricted provider/transport/resolver causes SHALL be serialized. This exception for actionable BYOK failures SHALL NOT invent a second error protocol or settle separately owned native failure-evidence behavior.

#### Scenario: Actionable credential shape failure
- **WHEN** a credential entry is missing apiKey or exceeds a bound
- **THEN** the invalid-request response SHALL identify the known field/capability without copying the entry or value

#### Scenario: Discovery unsupported for BYOK
- **WHEN** authenticated BYOK access requests catalog discovery
- **THEN** the host SHALL emit HTTP 400, invalid_request_error, invalid_request, param null and a fixed unsupported-discovery message

#### Scenario: Generic provider API failure
- **WHEN** a provider returns an arbitrary unclassified API/transport/internal failure
- **THEN** the runtime SHALL preserve existing safe classification without serializing unrestricted causes

#### Scenario: Unknown configured model
- **WHEN** configured catalog resolution reports an unknown public model
- **THEN** the handler SHALL return the fixed model-not-found document

#### Scenario: Client classification
- **WHEN** Go and pinned Vercel consume BYOK validation, exhaustion or discovery failures
- **THEN** status/type/retryability SHALL match the existing client categories without misclassifying provider rejection as failed Gateway authentication

### Requirement: Host-safe ProviderWire error writer
The `ai-gateway/providerwire/v4` package SHALL expose a narrow host-composition error writer for failures outside the language-model handler, including service authentication, permission, and discovery failures. Construction SHALL be non-fallible and SHALL accept no configuration. Callers SHALL select only closed authentication, permission, internal or unsupported-BYOK-discovery categories; they SHALL NOT supply public messages, causes, arbitrary status codes, error types, error codes, retryability, or byte limits.

The writer SHALL reuse package-owned fixed ProviderWire documents directly. It SHALL perform no runtime schema compilation or validation and no dynamic JSON encoding. Authentication SHALL emit the package's exact fixed 401 document, permission SHALL emit the exact fixed 403 document, internal SHALL emit the exact fixed 500 document, and an invalid category SHALL select that same internal document. The API SHALL not expose private DTOs or weaken the strict handler's existing failure bytes.

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
