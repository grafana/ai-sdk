## MODIFIED Requirements

### Requirement: Gateway app manifest
The Gateway SHALL be one Grafana app plugin whose manifest serves one API version with exactly six namespaced custom routes: `GET config`, `POST language-model`, `POST v1/chat/completions` and `POST v1/messages` under `namespaces/{namespace}/`, and `GET config` and `POST language-model` under `namespaces/{namespace}/modelcatalogs/{name}/`, plus two kinds, `ModelCatalog` and `Middleware`, with validation admission and no mutation or conversion hooks. It SHALL declare no other routes, kinds or hooks.

#### Scenario: Committed manifest declares only the three routes
- **WHEN** the committed generated manifest is inspected
- **THEN** it SHALL declare one version with only the four namespace routes and the two `ModelCatalog` subresource routes, and the `ModelCatalog` and `Middleware` kinds as its only kinds
- **AND** each catalog route SHALL declare a `get` access check on its own `modelcatalogs/<route>` subresource, and the namespace routes SHALL declare none

#### Scenario: Plugin identity is fixed
- **WHEN** the backend is built by any supported method, including plain `go build` in tests
- **THEN** it SHALL identify as the plugin ID declared in the plugin metadata

#### Scenario: Undeclared path or method
- **WHEN** a request targets another path under the plugin's group version that is not a declared kind, or a declared route with another method
- **THEN** the Gateway SHALL perform no discovery, selection or provider work for it

### Requirement: Route table matches the manifest
The production route table SHALL declare exactly the method and path pairs of the committed manifest's custom routes. A request that matches no entry, including another method on a declared path, SHALL perform no access decision, discovery, selection or provider work, and SHALL be labeled unmatched.

#### Scenario: Production table and manifest agree
- **WHEN** the production route table is compared with the committed manifest
- **THEN** both SHALL contain only `GET namespaces/{namespace}/config`, `POST namespaces/{namespace}/language-model`, `POST namespaces/{namespace}/v1/chat/completions`, `POST namespaces/{namespace}/v1/messages`, and `GET` `config` and `POST` `language-model` under `namespaces/{namespace}/modelcatalogs/{name}/`

#### Scenario: Undeclared path or method
- **WHEN** a request targets another path under the route prefix, or a declared path with another method, including `HEAD` on either `config` route
- **THEN** no catalog, selection or provider call SHALL occur, and the response SHALL NOT be a Gateway error document
