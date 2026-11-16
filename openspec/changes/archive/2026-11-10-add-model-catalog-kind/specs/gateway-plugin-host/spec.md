# Spec Delta

## RENAMED Requirements

- FROM: `### Requirement: Routes-only app plugin`
- TO: `### Requirement: Gateway app manifest`

## MODIFIED Requirements

### Requirement: Gateway app manifest
The Gateway SHALL be distributed as one Grafana app plugin whose app manifest serves one API version with exactly two namespaced custom routes, `GET namespaces/{namespace}/config` and `POST namespaces/{namespace}/language-model`, and exactly one kind, `ModelCatalog`, with validation admission and no mutation or conversion hooks. The manifest SHALL declare no other routes, kinds or hooks.

#### Scenario: Committed manifest declares only the two routes
- **WHEN** the committed generated manifest is inspected
- **THEN** it SHALL declare one version with only the two custom routes, and the `ModelCatalog` kind as its only kind

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
- **THEN** both SHALL contain only `GET namespaces/{namespace}/config` and `POST namespaces/{namespace}/language-model`

#### Scenario: Undeclared path or method
- **WHEN** a request targets another path under the route prefix, or a declared path with another method, including `HEAD` on `config`
- **THEN** no catalog, selection or provider call SHALL occur, and the response SHALL NOT be a Gateway error document

### Requirement: Protocol test host
Deterministic tests SHALL serve the Gateway through the real plugin protocol, from HTTP through the plugin route and admission RPCs to the Gateway handlers, with test-signed delegated tokens and fake native providers. Cross-language suites SHALL launch the built plugin executable as a plugin host does. The test host SHALL be test-only, SHALL NOT be shipped or documented as a supported server or as evidence of the platform's deadline value or draining, and MAY apply a route deadline.

#### Scenario: Test host exercises the protocol hop
- **WHEN** a client test sends a request to the test host
- **THEN** the request SHALL reach the Gateway through the same route protocol and identity delegation the plugin host uses, rather than calling Gateway handlers directly

#### Scenario: Real plugin process under test
- **WHEN** a cross-language suite starts the test host
- **THEN** the Gateway SHALL run as the built plugin executable, and health, metrics collection, relayed logs and stop SHALL be observed through the plugin protocol

#### Scenario: Legacy command evidence
- **WHEN** an existing requirement cites authenticated command evidence
- **THEN** that evidence SHALL be produced through this test host

#### Scenario: Test host is not shipped
- **WHEN** the plugin package is built
- **THEN** it SHALL contain no test host executable or configuration

#### Scenario: Manifest kinds in the test host
- **WHEN** a test creates, gets, updates or deletes an object of a kind the manifest declares
- **THEN** the test host SHALL store it in memory, send the plugin an admission review for each operation the kind declares, and refuse the write when the plugin denies it
- **AND** the test host SHALL NOT be cited as evidence of the platform's schema validation, pruning or authorization of kinds

#### Scenario: Secure values in the test host
- **WHEN** a test host write carries secure values
- **THEN** the test host SHALL apply the platform's inline semantics (create, remove, keep omitted keys, keep unchanged names) after admission, and return only names on reads

## ADDED Requirements

### Requirement: Kinds served by the plugin app
The plugin app SHALL manage each manifest kind with exactly the admission handlers the manifest declares, next to the route handler, and SHALL start no informers, watchers or reconcilers for them. An idle Gateway SHALL refuse every admission review, so writes fail closed on a host that delegates no requests to it.

#### Scenario: App and manifest agree
- **WHEN** the app is built from the committed manifest
- **THEN** every declared kind SHALL be managed and each declared admission capability SHALL have its handler, with no undeclared handler

#### Scenario: Admission through the plugin protocol
- **WHEN** the host sends an admission review for a `ModelCatalog` with a delegated token
- **THEN** the plugin SHALL authenticate it like a route call and answer with the catalog validator's decision

#### Scenario: Idle Gateway
- **WHEN** an idle Gateway receives an admission review
- **THEN** it SHALL refuse it without validating the object

### Requirement: Namespace routes unchanged by kinds
Declaring a kind SHALL NOT change the namespace `config` and `language-model` routes: their methods, access decisions, statuses, headers, bodies, streaming and Gateway error documents SHALL stay as they were with a routes-only manifest.

#### Scenario: Routes against Grafana with the kind declared
- **WHEN** the end-to-end suite calls the namespace routes on Grafana with the manifest that declares `ModelCatalog`
- **THEN** discovery, language-model error documents and undeclared-method responses SHALL match what the same tests observe with the routes-only manifest
