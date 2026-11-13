## MODIFIED Requirements

### Requirement: Gateway app manifest
The Gateway SHALL be distributed as one Grafana app plugin whose app manifest serves one API version with exactly two namespaced custom routes, `GET namespaces/{namespace}/config` and `POST namespaces/{namespace}/language-model`, and exactly two kinds, `ModelCatalog` and `Middleware`, each with validation admission and no mutation or conversion hooks. The manifest SHALL declare no other routes, kinds or hooks.

#### Scenario: Committed manifest declares only the two routes
- **WHEN** the committed generated manifest is inspected
- **THEN** it SHALL declare one version with only the two custom routes, and the `ModelCatalog` and `Middleware` kinds as its only kinds

#### Scenario: Plugin identity is fixed
- **WHEN** the backend is built by any supported method, including plain `go build` in tests
- **THEN** it SHALL identify as the plugin ID declared in the plugin metadata

#### Scenario: Undeclared path or method
- **WHEN** a request targets another path under the plugin's group version that is not a declared kind, or a declared route with another method
- **THEN** the Gateway SHALL perform no discovery, selection or provider work for it

### Requirement: Kinds served by the plugin app
The plugin app SHALL manage each manifest kind with exactly the admission handlers the manifest declares, next to the route handler, and SHALL start no informers, watchers or reconcilers for them. The `Middleware` validator SHALL decide with the public tenant types and the distribution's extension definitions. An idle Gateway SHALL refuse every admission review, so writes fail closed on a host that delegates no requests to it.

#### Scenario: App and manifest agree
- **WHEN** the app is built from the committed manifest
- **THEN** every declared kind SHALL be managed and each declared admission capability SHALL have its handler, with no undeclared handler

#### Scenario: Admission through the plugin protocol
- **WHEN** the host sends an admission review for a `ModelCatalog` with a delegated token
- **THEN** the plugin SHALL authenticate it like a route call and answer with the catalog validator's decision

#### Scenario: Middleware admission through the plugin protocol
- **WHEN** the host sends an admission review for a `Middleware` with a delegated token
- **THEN** the plugin SHALL authenticate it like a route call and answer with the decision for the public tenant types and the extension definitions the distribution passed

#### Scenario: Idle Gateway
- **WHEN** an idle Gateway receives an admission review
- **THEN** it SHALL refuse it without validating the object

### Requirement: Namespace routes unchanged by kinds
Declaring a kind SHALL NOT change the namespace `config` and `language-model` routes: their methods, access decisions, statuses, headers, bodies, streaming and Gateway error documents SHALL stay as they were with a routes-only manifest.

#### Scenario: Routes against Grafana with the kind declared
- **WHEN** the end-to-end suite calls the namespace routes on Grafana with the manifest that declares the Gateway's kinds
- **THEN** discovery, language-model error documents and undeclared-method responses SHALL match what the same tests observe with the routes-only manifest
