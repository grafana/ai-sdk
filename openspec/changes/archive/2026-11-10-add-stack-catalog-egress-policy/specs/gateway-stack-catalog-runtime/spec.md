## MODIFIED Requirements

### Requirement: Stack-catalog routing and fallback
Stack-catalog selections SHALL resolve canonical IDs and aliases within the catalog. They SHALL apply `models`, `order` and `only` and ordered fallback with the same rules, errors, deduplication and attempt bound as operator-catalog selections, and provider slugs SHALL be derived as for operator providers: an `openai-compatible` provider's `providerName` when set, otherwise the provider type.

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
- **THEN** the call SHALL succeed with two provider requests and an overview listing both attempts

#### Scenario: Composed chain above the bound
- **WHEN** routing controls compose more than 8 candidates
- **THEN** the call SHALL fail with the attempt-bound document, with zero constructed models and zero provider requests

#### Scenario: Compatible provider slug
- **WHEN** a catalog has an `openai-compatible` provider with `providerName: acme` and a call sends `only: ["acme"]`
- **THEN** the selection SHALL keep that provider's candidates, and `only: ["openai-compatible"]` SHALL exclude them
