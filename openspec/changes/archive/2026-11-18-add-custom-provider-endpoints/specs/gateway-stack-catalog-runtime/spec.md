## MODIFIED Requirements

### Requirement: Stored catalog checked on every request
Every stack-catalog selection SHALL check the stored spec with the stack-catalog provider policy, the custom endpoint list, the shared catalog shape checks and the key check, before resolving a model. A spec that fails, or a provider whose decrypted value is missing or empty, SHALL fail the call with the catalog configuration category before any provider request. The failure SHALL NOT carry check messages, spec identifiers or key values.

#### Scenario: Stored catalog fails the checks
- **WHEN** a stored spec has an unknown provider type or an alias collision
- **THEN** the call SHALL fail with the catalog configuration category and its fixed document, with zero provider requests

#### Scenario: Provider without a decrypted key
- **WHEN** a catalog provider has no decrypted value, or an empty one
- **THEN** the call SHALL fail with the catalog configuration category, with zero provider requests

#### Scenario: Failure does not echo the catalog
- **WHEN** a check fails on a spec whose provider and model names and keys are sentinel values
- **THEN** no sentinel SHALL appear in the response, the logs, the metrics or Agent Observability

#### Scenario: Catalog edited between requests
- **WHEN** a catalog is valid for one request and invalid for the next
- **THEN** the first SHALL succeed and the second SHALL fail with the catalog configuration category

#### Scenario: Endpoint no longer listed
- **WHEN** a stored catalog's custom `baseURL` is not in the current list
- **THEN** the call SHALL fail with the catalog configuration category, with zero provider requests

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
