# Spec Delta

## MODIFIED Requirements

### Requirement: Catalog route discovery
A catalog route's `config` SHALL list only the parent catalog's explicitly listed models with the configured route projection: canonical rows, aliases, primary and fallbacks, with each provider name as the provider instance and its provider type as the provider. It SHALL construct no model, make no provider request, and contain no key, secure value name or object metadata.

#### Scenario: Catalog with two models
- **WHEN** a caller with catalog access requests the catalog's `config`
- **THEN** the document SHALL list both models in ascending canonical-ID order, in the same shape as namespace route discovery

#### Scenario: Registered clients discover a catalog
- **WHEN** the pinned TypeScript client and the Go client use the catalog route as their base URL
- **THEN** each SHALL list the catalog's models without client changes

#### Scenario: No configured models
- **WHEN** a caller with catalog access requests `config` for a valid catalog with no models, with or without connected providers
- **THEN** the response SHALL be `200` with `{"models":[]}`, with no provider requests

#### Scenario: Catalog has an enabled provider
- **WHEN** a caller requests `config` for a catalog with a `serveAllModels` provider
- **THEN** the response SHALL have the same model rows as it would without the setting, and SHALL NOT expand to provider inventory
