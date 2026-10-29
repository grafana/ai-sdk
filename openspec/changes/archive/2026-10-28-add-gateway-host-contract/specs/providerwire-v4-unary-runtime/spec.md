# Spec Delta

## ADDED Requirements

### Requirement: ProviderWire host contract error writer
The ProviderWire package SHALL expose a non-fallible, unconfigured host error writer that implements the host contract's closed categories: permission, internal and unsupported-BYOK-discovery failures. Caller messages, causes or arbitrary fields SHALL NOT cross that API.

#### Scenario: Authenticated host error policy
- **WHEN** host middleware selects a closed error category
- **THEN** The `ai-gateway/providerwire/v4` package SHALL expose a narrow host-composition error writer for failures outside the language-model handler, including permission and discovery failures.
- **AND** Construction SHALL be non-fallible and SHALL accept no configuration.
- **AND** Callers SHALL select only the closed permission, internal or unsupported-BYOK-discovery categories; they SHALL NOT supply public messages, causes, arbitrary status codes, error types, error codes, retryability, or byte limits.
- **AND** The writer SHALL use the same protocol-owned public error definitions and typed HTTP encoder as the language-model handler, without optional execution metadata.
- **AND** It SHALL perform no runtime schema compilation or validation; encoding SHALL use only fixed package-owned values.
- **AND** Permission SHALL emit the exact fixed 403 document, internal SHALL emit the exact fixed 500 document, and an invalid category SHALL select that same internal document.
- **AND** The API SHALL not expose private DTOs or weaken the strict handler's existing failure bytes.

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

## MODIFIED Requirements

### Requirement: Closed host error categories and fixed document reuse
Callers SHALL select only the closed permission, internal or unsupported-BYOK-discovery categories, never public messages, causes, arbitrary statuses/types/codes, retryability or byte limits. Encoding SHALL use the handler's typed fixed definitions without schema compilation/validation or optional execution metadata.

#### Scenario: Closed host error categories and fixed document reuse
- **WHEN** host middleware selects permission rather than providing its own error message
- **THEN** the writer SHALL encode the shared fixed public definitions without accepting caller-controlled fields, compiling schemas or attaching optional execution metadata

#### Scenario: Unsupported discovery uses a closed category
- **WHEN** an authenticated BYOK caller requests discovery
- **THEN** the host SHALL select the fixed unsupported-discovery category without catalog access

### Requirement: Host error status bytes and private API boundary

Permission SHALL emit the package's exact fixed 403 document, internal SHALL emit the exact fixed 500 document, and an invalid category SHALL select that same internal document. The API SHALL not expose private DTOs or weaken the strict handler's existing failure bytes.

#### Scenario: Host error status bytes and private API boundary
- **WHEN** host middleware selects permission, internal or an invalid category
- **THEN** permission SHALL use the exact fixed 403 and internal/invalid the exact fixed 500, with no exposed DTOs or weakened handler failure bytes

## REMOVED Requirements

### Requirement: Host-safe ProviderWire error writer
**Reason**: The host no longer has an authentication category, because the platform authenticates callers before the plugin runs. The writer now implements the host contract's closed categories.
**Migration**: Use "ProviderWire host contract error writer", which keeps the permission, internal and BYOK-discovery documents byte-identical.
