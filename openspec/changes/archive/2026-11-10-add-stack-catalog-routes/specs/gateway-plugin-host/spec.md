## ADDED Requirements

### Requirement: Catalog route parent check
Before using a catalog route's decrypted values, the Gateway SHALL check that the route carries a parent object whose group, version, kind, namespace and name match the route's group, version, `ModelCatalog`, namespace and `{name}`. A missing or mismatched parent SHALL fail with the internal host error before the body is read, with no access decision, listing, selection or provider work.

#### Scenario: Parent names another catalog
- **WHEN** a catalog route for `team` receives a parent named `other`, or one from another namespace
- **THEN** the response SHALL be the internal host error, and no decrypted value SHALL reach selection or a provider

#### Scenario: Route without a parent
- **WHEN** a catalog route request carries no parent object
- **THEN** the response SHALL be the internal host error

#### Scenario: Parent whose spec does not decode
- **WHEN** the parent names the route's catalog but its spec does not decode as a `ModelCatalog` spec
- **THEN** language-model SHALL fail with the catalog configuration document and `config` with the catalog configuration host error

#### Scenario: Parent whose identity does not decode
- **WHEN** the parent is not JSON, or its API version, kind, namespace or name cannot be read
- **THEN** the response SHALL be the internal host error

## MODIFIED Requirements

### Requirement: Gateway app manifest
The Gateway SHALL be distributed as one Grafana app plugin whose app manifest serves one API version with exactly four namespaced custom routes, `GET` `config` and `POST` `language-model` under `namespaces/{namespace}/` and under `namespaces/{namespace}/modelcatalogs/{name}/`, and one kind, `ModelCatalog`, with validation admission and no mutation or conversion hooks. The manifest SHALL declare no other routes, kinds or hooks.

#### Scenario: Committed manifest declares only the two routes
- **WHEN** the committed generated manifest is inspected
- **THEN** it SHALL declare one version with only the two namespace routes and the two `ModelCatalog` subresource routes, and the `ModelCatalog` kind as its only kind
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
- **THEN** both SHALL contain only `GET` `config` and `POST` `language-model` under `namespaces/{namespace}/` and under `namespaces/{namespace}/modelcatalogs/{name}/`

#### Scenario: Undeclared path or method
- **WHEN** a request targets another path under the route prefix, or a declared path with another method, including `HEAD` on either `config` route
- **THEN** no catalog, selection or provider call SHALL occur, and the response SHALL NOT be a Gateway error document

### Requirement: Closed host-error categories
Host code SHALL report its own failures through a closed set of host-error categories: permission, internal, BYOK discovery unsupported and catalog configuration. Each adapter SHALL write a fixed document for each category. The host SHALL NOT define an authentication category, because the platform authenticates callers before the plugin runs.

#### Scenario: ProviderWire host documents are unchanged
- **WHEN** ProviderWire writes the permission, internal or BYOK discovery unsupported host error
- **THEN** the status, headers and body SHALL be byte-identical to the documents written before this change

#### Scenario: Catalog configuration host error
- **WHEN** catalog route discovery finds a parent catalog that fails its checks or lacks a provider key
- **THEN** ProviderWire SHALL write the same fixed document as the runtime's catalog configuration category

#### Scenario: Adapter-owned format
- **WHEN** a host failure occurs on an entry whose adapter is not ProviderWire
- **THEN** the response SHALL use that adapter's document for the category, not ProviderWire's

### Requirement: Host-owned discovery listing
The host SHALL own the discovery decision and listing. Configured access SHALL list the operator catalog, catalog access its route's parent catalog, BYOK access SHALL get the BYOK-discovery-unsupported host error without listing, and other callers the permission error. A listing that panics or fails, or a projection that fails, SHALL produce the internal host error. The adapter SHALL own only the projection of listed models into its document.

#### Scenario: Configured discovery document is unchanged
- **WHEN** a configured caller requests `config`
- **THEN** the ProviderWire discovery document SHALL be byte-identical to the one written before this change, and the configured-discovery Go and TypeScript suites SHALL pass unchanged

#### Scenario: BYOK discovery
- **WHEN** a BYOK caller requests `config` in `cloud` mode
- **THEN** the response SHALL be the fixed BYOK discovery unsupported document, with zero catalog listings

#### Scenario: Catalog route discovery
- **WHEN** a caller with catalog access requests a catalog route's `config`
- **THEN** the response SHALL list the parent catalog's models, and a parent that fails its checks SHALL get the catalog configuration host error

#### Scenario: Listing panics
- **WHEN** the catalog lister panics or returns an error
- **THEN** the response SHALL be the internal host error, with no partial document

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

#### Scenario: Subresource routes in the test host
- **WHEN** a test calls a path below a stored object of a declared kind
- **THEN** the test host SHALL forward it with the object as the route's parent: its resource, name, resource version, JSON and the decrypted values it holds for the object's secure keys
- **AND** a missing object SHALL get `404` without a route call, and the test host SHALL NOT be cited as evidence of Grafana's declared access checks or decryption
