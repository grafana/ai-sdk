## MODIFIED Requirements

### Requirement: Closed host error categories and fixed document reuse
Callers SHALL select only the closed permission, internal, unsupported-BYOK-discovery or catalog configuration categories, never public messages, causes, arbitrary statuses/types/codes, retryability or byte limits. Encoding SHALL use the handler's typed fixed definitions without schema compilation/validation or optional execution metadata.

#### Scenario: Closed host error categories and fixed document reuse
- **WHEN** host middleware selects permission rather than providing its own error message
- **THEN** the writer SHALL encode the shared fixed public definitions without accepting caller-controlled fields, compiling schemas or attaching optional execution metadata

#### Scenario: Unsupported discovery uses a closed category
- **WHEN** an authenticated BYOK caller requests discovery
- **THEN** the host SHALL select the fixed unsupported-discovery category without catalog access

#### Scenario: Catalog configuration host error
- **WHEN** the host selects the catalog configuration category
- **THEN** the writer SHALL emit the fixed 424 `failed_dependency` document of the runtime's catalog configuration category
