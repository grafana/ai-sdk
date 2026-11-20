## MODIFIED Requirements

### Requirement: Stack-catalog routing and fallback
Stack-catalog selections SHALL resolve canonical IDs and aliases within the catalog. They SHALL apply `models`, `order` and `only` and ordered fallback with the same rules, errors, deduplication and attempt bound as operator-catalog selections, and provider slugs SHALL be the provider types.

#### Scenario: Alias call
- **WHEN** a call requests an alias of a catalog model
- **THEN** the selection SHALL use that model's canonical ID and candidates

#### Scenario: Unknown model
- **WHEN** a call requests an ID that is neither a model nor an alias in the catalog
- **THEN** the call SHALL fail with the model not found category, with zero provider requests

#### Scenario: No configured models
- **WHEN** a valid catalog call requests a model from a checked catalog with no models, with or without connected providers
- **THEN** the call SHALL fail with the existing model not found category (`404 model_not_found`), without constructing a candidate, using another catalog, or contacting a provider

#### Scenario: Unknown models entry
- **WHEN** a `gateway.models` entry names no model or alias in the catalog
- **THEN** the call SHALL fail with model not found and the fallback-model reason, with zero provider requests

#### Scenario: Primary fails and the fallback succeeds
- **WHEN** the primary candidate fails with a fallback-eligible error and the fallback succeeds
- **THEN** the call SHALL succeed with two provider requests and an overview listing both attempts

#### Scenario: Composed chain above the bound
- **WHEN** routing controls compose more than 8 candidates
- **THEN** the call SHALL fail with the attempt-bound document, with zero constructed models and zero provider requests

### Requirement: Closed health results for the saved catalog
On an accepted check the response SHALL be non-cacheable `200` JSON with `providers` mapping provider names to statuses, and `models` mapping canonical model IDs to a `primary` status and ordered `fallback` statuses. Statuses SHALL use only `ok`, `invalidKey`, `unknownModel`, `unreachable`, `refused`, and `notChecked`. Aliases SHALL NOT produce separate model entries.

#### Scenario: Catalog without providers
- **WHEN** a health check is admitted for a valid catalog without providers
- **THEN** it SHALL return `200` with `{"providers":{},"models":{}}` and make no provider requests

#### Scenario: Provider with no configured targets
- **WHEN** a health check probes a connected provider that is neither a primary nor a fallback of any configured model
- **THEN** it SHALL check the first listing page with the same provider outcomes as other providers, SHALL NOT request another page, and SHALL NOT emit any target statuses for that provider

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

### Requirement: List one saved provider's models
The `available-models` action SHALL accept exactly one `provider` query parameter naming a provider in the checked parent catalog and no body. It SHALL run the same stored-catalog preflight as selection and health before serving a listing, use only the named provider's saved key and validated destination, and never accept draft configuration or credentials. The action SHALL NOT run inference or modify the catalog.

#### Scenario: List one of two accounts of the same type
- **WHEN** a valid caller asks for one named provider in a saved catalog
- **THEN** the action SHALL use that provider's account only and SHALL NOT substitute another provider's key or ambient credentials

#### Scenario: Browse before adding any models
- **WHEN** a valid caller asks for a saved provider in a catalog that has no configured models
- **THEN** the action SHALL list that provider's inventory using the usual listing rules

#### Scenario: Invalid input
- **WHEN** the action receives a body, a missing or invalid selector, duplicate selectors, or extra parameters
- **THEN** it SHALL return a fixed `400` with no provider request

#### Scenario: Unknown provider
- **WHEN** the selector is valid but absent from an otherwise valid catalog, including one with no providers
- **THEN** the action SHALL return a fixed `404` with no provider request

#### Scenario: Invalid stored configuration
- **WHEN** the saved catalog fails shared shape, endpoint or key checks
- **THEN** the action SHALL return a fixed `424` without contacting the provider
