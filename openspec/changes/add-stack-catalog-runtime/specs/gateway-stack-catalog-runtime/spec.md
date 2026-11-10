# gateway-stack-catalog-runtime Specification

## Purpose

Define how the Gateway builds and selects models from one stored stack catalog and its decrypted provider keys, for one request, without operator configuration or server-held keys.

## ADDED Requirements

### Requirement: Stack-catalog selection input
A stack-catalog selection SHALL be built from three inputs: one catalog's spec, its decrypted secure values keyed by provider name, and the catalog's name. It SHALL also use the requested model ID and the call's `gateway` controls. It SHALL read no other catalog, operator catalog entry, operator provider, plugin setting or ambient credential. Its source SHALL be `stack-catalog`.

#### Scenario: Catalog call
- **WHEN** a stack-catalog selection succeeds
- **THEN** the selection SHALL have source `stack-catalog`, the requested model's canonical ID in the catalog, and the catalog's name

#### Scenario: Operator catalog has the same model ID
- **WHEN** the operator catalog and the stack catalog both define `assistant` with different providers
- **THEN** a stack-catalog selection of `assistant` SHALL attempt only the stack catalog's candidates, with the stack catalog's keys

### Requirement: Stored catalog checked on every request
Every stack-catalog selection SHALL check the stored spec with the stack-catalog provider policy, the shared catalog shape checks and the key check, before resolving a model. A spec that fails, or a provider whose decrypted value is missing or empty, SHALL fail the call with the catalog configuration category before any provider request. The failure SHALL NOT carry check messages, spec identifiers or key values.

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

### Requirement: Only needed candidates are constructed
A stack-catalog selection SHALL construct provider models only for the candidates its composed attempt chain contains. Those come from the requested model and any `gateway.models` entries, filtered by `only` and ordered by `order`. It SHALL NOT construct candidates of other catalog models, and SHALL keep no built model after the call.

#### Scenario: Catalog with many models
- **WHEN** a catalog defines 50 models and a call requests one model with one fallback
- **THEN** exactly two provider models SHALL be constructed

#### Scenario: Only filter excludes candidates
- **WHEN** a call requests a model with an `anthropic` primary and an `openai` fallback and sends `only: ["openai"]`
- **THEN** only the `openai` candidate SHALL be constructed

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
- **THEN** the call SHALL succeed with two provider requests and an overview listing both attempts

#### Scenario: Composed chain above the bound
- **WHEN** routing controls compose more than 8 candidates
- **THEN** the call SHALL fail with the attempt-bound document, with zero constructed models and zero provider requests

### Requirement: BYOK controls refused on stack catalogs
A stack-catalog selection SHALL refuse `gateway.byok` and any other unsupported control with the unsupported gateway control category, before checking the stored catalog and before any provider request. It SHALL never fall back to the operator catalog, server-held provider keys or ambient credentials.

#### Scenario: Request brings BYOK accounts
- **WHEN** a stack-catalog call sends valid `gateway.byok` accounts
- **THEN** the call SHALL fail with the unsupported gateway control document, with zero provider requests

#### Scenario: Catalog provider key is missing and the operator has one
- **WHEN** a stack catalog's provider has no decrypted key and the operator catalog has a provider of the same type with a key
- **THEN** the call SHALL fail with the catalog configuration category, and the operator key SHALL NOT be used

### Requirement: Stack-catalog execution overview
A stack-catalog call SHALL publish the same execution overview as an operator-catalog call: the requested ID, the canonical model ID, and each attempt's provider name, provider type and backend model ID.

#### Scenario: Direct success
- **WHEN** a stack-catalog call succeeds on its primary candidate
- **THEN** the overview SHALL list one selected attempt with the catalog's provider name as provider instance
