# gateway-stack-catalog-runtime Specification

## Purpose
Define how the Gateway builds and selects models from one stored stack catalog and its decrypted provider keys, for one request, without operator configuration or server-held keys.

## Requirements

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

### Requirement: No private attempt records for stack catalogs
Stack-catalog calls use customer-owned keys, and like BYOK calls they SHALL write no private physical attempt records. Their per-attempt evidence SHALL be the execution overview returned to the caller.

#### Scenario: Fallback on a stack catalog
- **WHEN** a stack-catalog call's primary fails and its fallback succeeds
- **THEN** no physical attempt record SHALL be written, and the overview SHALL list both attempts

### Requirement: Catalog language-model route
A catalog route's `language-model` call SHALL select from its parent catalog as a stack-catalog selection: the parent's spec, the decrypted values the platform passed with the request, keyed by provider name, and the parent's name. The Gateway SHALL keep no catalog, key or built model between requests, so the next request after a catalog edit uses the edited catalog and keys.

#### Scenario: Decrypted key reaches the provider
- **WHEN** a catalog route call selects a model of an `anthropic` provider
- **THEN** the native request SHALL carry that provider's decrypted value as its API key

#### Scenario: Catalog edited between requests
- **WHEN** a provider's key or a model's primary changes, and the platform passes the new object and values
- **THEN** the next call SHALL use the new key or model

#### Scenario: Unary and streaming calls
- **WHEN** a registered client calls a catalog route with the catalog as its base URL, unary or streaming
- **THEN** the response SHALL use the same ProviderWire format as a namespace route call

### Requirement: Health checks use only a saved catalog
The health action SHALL accept no body or query parameters. It SHALL use only the checked parent catalog's spec and decrypted keys, applying the same stored-catalog checks, endpoint policy and resolved destinations as selection before any probe. It SHALL NOT run on save, use caller-supplied credentials, or consult another catalog or ambient credentials.

#### Scenario: Invalid stored catalog
- **WHEN** the saved catalog fails shape, endpoint policy or decrypted key checks
- **THEN** the whole check SHALL return a fixed catalog configuration error (`424`) with no provider requests

#### Scenario: User supplies a draft
- **WHEN** a valid caller submits a body or query parameters to the health action
- **THEN** the request SHALL fail with a fixed `400` before any provider requests

### Requirement: Health checks use provider model listings
The action SHALL use read-only provider model listings, not inference. A valid listing response SHALL confirm that the listing request was accepted, and only an exact match on the configured backend model ID SHALL count as listed. A match SHALL NOT attest to inference, quota, billing, or capability support.

#### Scenario: Catalog has primary and fallback models
- **WHEN** a catalog has primary and fallback targets sharing a provider
- **THEN** both SHALL be evaluated independently using the same listing traversal for that provider, not a call per model

### Requirement: Closed health results for the saved catalog
On an accepted check the response SHALL be non-cacheable `200` JSON with `providers` mapping provider names to statuses, and `models` mapping canonical model IDs to a `primary` status and ordered `fallback` statuses. Statuses SHALL use only `ok`, `invalidKey`, `unknownModel`, `unreachable`, `refused`, and `notChecked`. Aliases SHALL NOT produce separate model entries.

#### Scenario: Accepted native listing
- **WHEN** the official endpoint returns a valid model listing
- **THEN** its provider status SHALL be `ok` and listed targets SHALL be `ok`
- **AND** unmatched targets SHALL be `unknownModel` only if the listing is complete, otherwise `notChecked`

#### Scenario: Accepted listing on a custom endpoint
- **WHEN** an allowed custom endpoint returns a valid listing
- **THEN** its provider and listed targets SHALL be `ok`, but unmatched targets SHALL be `notChecked` because completeness is not assured

#### Scenario: Listing fails before any valid page
- **WHEN** the provider returns `401`, denies or throttles the request, or fails at the transport level or with an upstream server error
- **THEN** the provider status SHALL be `invalidKey`, `refused`, or `unreachable` respectively, and the corresponding targets SHALL be `notChecked`

#### Scenario: Custom endpoint does not implement listing
- **WHEN** an allowed custom endpoint responds with `404`, `405`, or `501` to its listing request
- **THEN** its provider and all corresponding targets SHALL be `notChecked`

#### Scenario: Later page fails
- **WHEN** listing fails after at least one valid page
- **THEN** the provider and the already observed targets SHALL remain `ok` and unobserved targets SHALL be `notChecked`

### Requirement: Health is bounded and secret-safe
The action SHALL limit providers, pages, decoded bytes, concurrency, and elapsed time, including response reads, and apply per-catalog and process-wide admission limits. It SHALL NOT follow redirects, retry failed probes, or queue refused runs. Health responses and telemetry SHALL NOT include key material, outbound URLs, or upstream bodies, headers, or error text.

#### Scenario: Work exceeds the check budget
- **WHEN** a provider cannot start within the probe count or time budget
- **THEN** it and all corresponding targets SHALL be `notChecked`; completed results SHALL still be returned

#### Scenario: Too many health requests
- **WHEN** the catalog already has a check running or the applicable admission budget is exhausted
- **THEN** the action SHALL return a fixed `429` with zero provider requests for that check

#### Scenario: Provider echoes a secret
- **WHEN** an upstream error or response with a malformed listing contains a secret in its text
- **THEN** neither that text nor the secret SHALL appear in any health response, log, metric or trace
