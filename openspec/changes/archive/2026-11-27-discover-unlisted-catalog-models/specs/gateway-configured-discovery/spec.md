## MODIFIED Requirements

### Requirement: Catalog route discovery
A catalog route's `config` SHALL preserve the parent catalog's explicitly listed models with the configured route projection: canonical rows, aliases, primary and fallbacks, with each provider name as the provider instance and its provider slug as the provider: an `openai-compatible` provider's `providerName` when set, otherwise its type. It SHALL construct no model and contain no key, secure value name or object metadata. The successful document SHALL use `Cache-Control: no-store`.

#### Scenario: Catalog with two models
- **WHEN** a caller with catalog access requests the catalog's `config`
- **THEN** the document SHALL list both models in ascending canonical-ID order, in the same shape as namespace route discovery

#### Scenario: Registered clients discover a catalog
- **WHEN** the pinned TypeScript client and the Go client use the catalog route as their base URL
- **THEN** each SHALL list the catalog's models without client changes

#### Scenario: Compatible provider in a catalog
- **WHEN** a catalog model's primary is an `openai-compatible` provider named `local` with `providerName: acme`
- **THEN** its candidate SHALL list provider instance `local` and provider `acme`, as namespace route discovery does for an operator provider

#### Scenario: No configured models
- **WHEN** a caller with catalog access requests `config` for a valid catalog with no listed models and no provider serving unlisted models
- **THEN** the response SHALL be `200` with `{"models":[]}`, with no provider requests

#### Scenario: Catalog has an enabled provider
- **WHEN** a caller requests `config` for a catalog with a `serveAllModels` provider
- **THEN** discovery SHALL also list qualified selectors from that provider's saved account inventory, subject to eligibility and the discovery limits

#### Scenario: Inventory contains a blocked or shadowed selector
- **WHEN** an inventory ID is blocked for its provider, is invalid for an unlisted selector, or forms a qualified selector matching a listed ID or alias
- **THEN** discovery SHALL NOT add a row for that ID and SHALL preserve the listed route

#### Scenario: Qualified row projection
- **WHEN** discovery adds a qualified selector
- **THEN** the row's ID and specification model ID SHALL be that selector, with one primary identifying the same provider instance, slug, and literal backend ID as invocation, and no aliases or fallbacks

## ADDED Requirements

### Requirement: Catalog provider discovery is bounded and best effort
Discovery SHALL list only opted-in providers using the checked catalog's keys and the bounded provider listing transport. It SHALL share the listing admission limits, probe at most eight providers within two seconds, and retain configured rows within the existing 1,024-ID budget including aliases. Inventory SHALL NOT be cached or required for invocation.

#### Scenario: Listing is unavailable or incomplete
- **WHEN** a provider fails, returns a partial listing, or a discovery budget is exhausted
- **THEN** discovery SHALL still return all configured rows and any eligible IDs already collected within the budget, without upstream error details

#### Scenario: Inventory echoes a key
- **WHEN** a provider lists an ID that contains a saved key or its header form
- **THEN** discovery SHALL NOT publish the ID or follow a continuation cursor containing that key
