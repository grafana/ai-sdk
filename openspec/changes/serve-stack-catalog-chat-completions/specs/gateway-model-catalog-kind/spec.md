## MODIFIED Requirements

### Requirement: Catalog roles
The manifest SHALL declare `aigateway:catalogreader` (viewer permission set on `ModelCatalog`, bound to Viewer), `aigateway:catalogwriter` (editor set, bound to Admin) and `aigateway:modelinvoker` (`get` on `ModelCatalog`, `ModelCatalog/language-model`, `ModelCatalog/config` and `ModelCatalog/chat-completions`, bound to Editor). Gateway role names SHALL use lowercase letters and digits only.

#### Scenario: Viewer
- **WHEN** a Viewer or Editor gets or lists catalogs in their namespace
- **THEN** the request SHALL be allowed, and their create, update and delete requests SHALL be refused

#### Scenario: Admin
- **WHEN** an Admin creates, updates or deletes a catalog
- **THEN** authorization SHALL allow the request, subject to admission

#### Scenario: Invoker
- **WHEN** an Editor or Admin calls a catalog route of a catalog in their namespace
- **THEN** Grafana's declared check SHALL allow it, and a Viewer's call SHALL be refused with `403` before the plugin is called
