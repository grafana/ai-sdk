# Stack Catalog Runtime Delta

## ADDED Requirements

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
