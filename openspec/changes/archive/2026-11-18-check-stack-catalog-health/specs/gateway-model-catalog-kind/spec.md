# Model Catalog Kind Delta

## MODIFIED Requirements

### Requirement: Catalog roles
The manifest SHALL declare `aigateway:catalogreader` (viewer permission set on `ModelCatalog`, bound to Viewer), `aigateway:catalogwriter` (editor set on `ModelCatalog` and `get` on `ModelCatalog/health`, bound to Admin) and `aigateway:modelinvoker` (`get` on `ModelCatalog`, `ModelCatalog/language-model`, `ModelCatalog/config` and `ModelCatalog/messages`, bound to Editor). Gateway role names SHALL use lowercase letters and digits only.

#### Scenario: Viewer
- **WHEN** a Viewer or Editor gets or lists catalogs in their namespace
- **THEN** the request SHALL be allowed, and their create, update and delete requests SHALL be refused

#### Scenario: Admin
- **WHEN** an Admin creates, updates or deletes a catalog
- **THEN** authorization SHALL allow the request, subject to admission

#### Scenario: Invoker
- **WHEN** an Editor or Admin calls `language-model`, `config` or `v1/messages` on a catalog in their namespace
- **THEN** Grafana's declared check SHALL allow it, and a Viewer's call SHALL be refused with `403` before the plugin is called

#### Scenario: Authorized health check
- **WHEN** a caller has access to the parent and `get` on `modelcatalogs/health`
- **THEN** Grafana's declared check SHALL allow the health action regardless of the caller's org role, subject to the plugin's catalog caller and parent checks

#### Scenario: Reader or invoker without health access
- **WHEN** a caller can read the parent catalog but does not have `get` on `modelcatalogs/health`
- **THEN** Grafana SHALL return `403` before decrypting the parent's keys or calling the plugin
