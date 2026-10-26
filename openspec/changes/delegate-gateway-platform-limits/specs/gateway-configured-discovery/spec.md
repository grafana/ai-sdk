## MODIFIED Requirements

### Requirement: Atomic discovery resource and consistency limits
Configuration loading SHALL validate route semantics before readiness: at most 1,024 callable public IDs including aliases, 8 candidates per route (the shared provider attempt bound under gateway-resource-policy), 128 aliases per route and 2,048 UTF-8 bytes per identity/display string. The existing 1–128 ASCII public-ID grammar SHALL remain unchanged. Required identities and names SHALL be nonblank; strings SHALL be valid UTF-8; optional description SHALL preserve existing empty/absent semantics.

#### Scenario: Server policy dimensions reach their boundaries
- **WHEN** a configured catalog reaches an exact public-ID, candidate, alias or string ceiling
- **THEN** configuration loading SHALL accept it if semantic constraints hold
- **WHEN** any policy dimension exceeds its ceiling by one
- **THEN** configuration loading SHALL fail before readiness

#### Scenario: Invalid routes fail startup
- **WHEN** configuration contains invalid IDs, duplicate aliases/candidate tuples, canonical/alias collisions or blank required identifiers
- **THEN** startup SHALL fail before serving requests or provider inference
- **AND** response building SHALL NOT repeat those checks

#### Scenario: Configured catalog exceeds the former response budget
- **WHEN** a valid configured discovery document exceeds 1 MiB after JSON encoding and escaping
- **THEN** the server SHALL serve every visible configured row without a discovery response-size rejection

#### Scenario: Consumers do not duplicate configuration policy
- **WHEN** a document fits the consumer byte budget and decodes into its typed metadata
- **THEN** the consumer SHALL retain supplied facts without checking route semantics, uniqueness or numeric configuration policy
- **WHEN** the document exceeds the consumer byte budget by one
- **THEN** the consumer SHALL fail without returning any rows

#### Scenario: Escaped lone surrogates use standard JSON semantics
- **WHEN** a display or candidate string contains an escaped lone UTF-16 surrogate
- **THEN** Go SHALL decode it as U+FFFD and TS SHALL retain its standard decoded UTF-16 value

#### Scenario: Client receives a late type error
- **WHEN** a bounded document contains valid initial rows followed by a field that cannot decode into its declared type
- **THEN** configured access SHALL fail atomically without exposing the initial rows

