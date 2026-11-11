## MODIFIED Requirements

### Requirement: Catalog route discovery
A catalog route's `config` SHALL list the parent catalog's models with the configured route projection: canonical rows, aliases, primary and fallbacks, with each provider name as the provider instance and its provider slug as the provider: an `openai-compatible` provider's `providerName` when set, otherwise its type. It SHALL construct no model, make no provider request, and contain no key, secure value name or object metadata.

#### Scenario: Catalog with two models
- **WHEN** a caller with catalog access requests the catalog's `config`
- **THEN** the document SHALL list both models in ascending canonical-ID order, in the same shape as namespace route discovery

#### Scenario: Registered clients discover a catalog
- **WHEN** the pinned TypeScript client and the Go client use the catalog route as their base URL
- **THEN** each SHALL list the catalog's models without client changes

#### Scenario: Compatible provider in a catalog
- **WHEN** a catalog model's primary is an `openai-compatible` provider named `local` with `providerName: acme`
- **THEN** its candidate SHALL list provider instance `local` and provider `acme`, as namespace route discovery does for an operator provider
