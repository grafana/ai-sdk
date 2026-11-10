## ADDED Requirements

### Requirement: Stored catalogs serve only their catalog routes
A stored `ModelCatalog` SHALL be discoverable and invocable only through its own catalog routes. It SHALL NOT change discovery, model selection, the operator catalog or BYOK on the namespace routes.

#### Scenario: Catalog stored next to the operator catalog
- **WHEN** a namespace holds a valid catalog
- **THEN** its namespace routes SHALL list and invoke the operator catalog exactly as without it, and the catalog's routes SHALL list and invoke only the catalog

## MODIFIED Requirements

### Requirement: Catalog roles
The manifest SHALL declare `aigateway:catalogreader` (viewer permission set on `ModelCatalog`, bound to Viewer), `aigateway:catalogwriter` (editor set, bound to Admin) and `aigateway:modelinvoker` (`get` on `ModelCatalog`, `ModelCatalog/language-model` and `ModelCatalog/config`, bound to Editor). Gateway role names SHALL use lowercase letters and digits only.

#### Scenario: Viewer
- **WHEN** a Viewer or Editor gets or lists catalogs in their namespace
- **THEN** the request SHALL be allowed, and their create, update and delete requests SHALL be refused

#### Scenario: Admin
- **WHEN** an Admin creates, updates or deletes a catalog
- **THEN** authorization SHALL allow the request, subject to admission

#### Scenario: Invoker
- **WHEN** an Editor or Admin calls either catalog route of a catalog in their namespace
- **THEN** Grafana's declared check SHALL allow it, and a Viewer's call SHALL be refused with `403` before the plugin is called

### Requirement: Catalog evidence against Grafana
Roles, folderless saves, secure storage and reads, updates, refusals, secret-safe errors and the catalog routes SHALL be proven by end-to-end tests against a running Grafana with the built plugin. The shape, policy, key and merge checks SHALL also have unit tests.

#### Scenario: End-to-end suite
- **WHEN** the plugin end-to-end suite runs against the pinned Grafana image
- **THEN** it SHALL exercise each behavior in this capability through Grafana's API

## REMOVED Requirements

### Requirement: Stored catalogs have no runtime effect
**Reason**: The catalog routes now serve stored catalogs.
**Migration**: "Stored catalogs serve only their catalog routes" keeps the namespace routes unaffected.

