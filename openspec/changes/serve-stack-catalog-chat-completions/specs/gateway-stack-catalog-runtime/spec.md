## MODIFIED Requirements

### Requirement: Stack-catalog routing and fallback
Stack-catalog selections SHALL resolve canonical IDs and aliases within the catalog. They SHALL apply `models`, `order` and `only` and ordered fallback with the same rules, errors, deduplication and attempt bound as operator-catalog selections, and provider slugs SHALL be the provider types.

#### Scenario: Alias call
- **WHEN** a call requests an alias of a catalog model
- **THEN** the selection SHALL use that model's canonical ID and candidates

#### Scenario: Unknown model
- **WHEN** a call requests an ID that is neither a model nor an alias in the catalog
- **THEN** the call SHALL fail with the model not found category, with zero provider requests

#### Scenario: Unknown models entry
- **WHEN** a `gateway.models` entry names no model or alias in the catalog
- **THEN** the call SHALL fail with model not found and the fallback-model reason, with zero provider requests

#### Scenario: Primary fails and the fallback succeeds
- **WHEN** the primary candidate fails with a fallback-eligible error and the fallback succeeds
- **THEN** the call SHALL succeed with two provider requests; for ProviderWire the overview SHALL list both attempts

#### Scenario: Composed chain above the bound
- **WHEN** routing controls compose more than 8 candidates
- **THEN** the call SHALL fail with the attempt-bound document, with zero constructed models and zero provider requests

### Requirement: Stack-catalog execution overview
A stack-catalog ProviderWire call SHALL publish the same execution overview as an operator-catalog call: the requested ID, the canonical model ID, and each attempt's provider name, provider type and backend model ID. Chat calls SHALL NOT publish that overview.

#### Scenario: Direct success
- **WHEN** a stack-catalog ProviderWire call succeeds on its primary candidate
- **THEN** the overview SHALL list one selected attempt with the catalog's provider name as provider instance

### Requirement: No private attempt records for stack catalogs
Stack-catalog calls use customer-owned keys, and like BYOK calls they SHALL write no private physical attempt records. For ProviderWire the execution overview returned to the caller SHALL provide per-attempt evidence.

#### Scenario: Fallback on a stack catalog
- **WHEN** a stack-catalog call's primary fails and its fallback succeeds
- **THEN** no physical attempt record SHALL be written, and a ProviderWire response's overview SHALL list both attempts
