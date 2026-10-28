# Spec Delta

## ADDED Requirements

### Requirement: Adapter route table
The Gateway SHALL compose its routes from one table. Each entry SHALL declare a method, a path under the namespaced route prefix, a fixed telemetry label, an adapter handler and that adapter's host-error writer. Every entry SHALL get the shared access decision and request telemetry. Adding an entry SHALL NOT require changes to dispatch, access, settings, catalog, discovery-listing or telemetry code.

#### Scenario: Entry receives host composition
- **WHEN** a request matches an entry's method and path
- **THEN** the access decision SHALL run before the entry's handler, and the request observation SHALL carry that entry's label

#### Scenario: Refused caller on any entry
- **WHEN** access policy refuses a caller on any entry
- **THEN** that entry's host-error writer SHALL write the permission document, and the Gateway SHALL NOT read the request body or call the handler, the catalog or a provider

#### Scenario: No Gateway body bound on entries
- **WHEN** an accepted request on any entry carries a body larger than the former Gateway body bound
- **THEN** the entry's handler SHALL receive the whole body, and only the platform's route message size SHALL bound it

#### Scenario: Invalid table
- **WHEN** the table has an entry with an empty label, a missing handler or writer, or the same method and path as another entry
- **THEN** Gateway construction SHALL fail before any request is served

### Requirement: Route table matches the manifest
The production route table SHALL declare exactly the method and path pairs of the committed manifest. A request that matches no entry, including another method on a declared path, SHALL perform no access decision, discovery, selection or provider work, and SHALL be labeled unmatched.

#### Scenario: Production table and manifest agree
- **WHEN** the production route table is compared with the committed manifest
- **THEN** both SHALL contain only `GET namespaces/{namespace}/config` and `POST namespaces/{namespace}/language-model`

#### Scenario: Undeclared path or method
- **WHEN** a request targets another path under the route prefix, or a declared path with another method, including `HEAD` on `config`
- **THEN** no catalog, selection or provider call SHALL occur, and the response SHALL NOT be a Gateway error document

### Requirement: Closed host-error categories
Host code SHALL report its own failures through a closed set of host-error categories: permission, internal and BYOK discovery unsupported. Each adapter SHALL write a fixed document for each category. The host SHALL NOT define an authentication category, because the platform authenticates callers before the plugin runs.

#### Scenario: ProviderWire host documents are unchanged
- **WHEN** ProviderWire writes the permission, internal or BYOK discovery unsupported host error
- **THEN** the status, headers and body SHALL be byte-identical to the documents written before this change

#### Scenario: Adapter-owned format
- **WHEN** a host failure occurs on an entry whose adapter is not ProviderWire
- **THEN** the response SHALL use that adapter's document for the category, not ProviderWire's

### Requirement: Host-owned discovery listing
The host SHALL own the discovery decision and listing. Configured access SHALL list the catalog under the request context, BYOK access SHALL get the BYOK-discovery-unsupported host error without listing, and other callers SHALL get the permission error. A listing that panics or fails, or a projection that fails, SHALL produce the internal host error. The adapter SHALL own only the projection of listed models into its document.

#### Scenario: Configured discovery document is unchanged
- **WHEN** a configured caller requests `config`
- **THEN** the ProviderWire discovery document SHALL be byte-identical to the one written before this change, and the configured-discovery Go and TypeScript suites SHALL pass unchanged

#### Scenario: BYOK discovery
- **WHEN** a BYOK caller requests `config` in `cloud` mode
- **THEN** the response SHALL be the fixed BYOK discovery unsupported document, with zero catalog listings

#### Scenario: Listing panics
- **WHEN** the catalog lister panics or returns an error
- **THEN** the response SHALL be the internal host error, with no partial document

### Requirement: Second-adapter composition evidence
Tests SHALL register a test-only adapter, which is not ProviderWire, on an extra route-table entry next to the production entries. This SHALL require no changes to dispatch, access, settings, catalog, discovery-listing or telemetry code. The test adapter SHALL NOT be part of the built plugin.

#### Scenario: Test adapter on an extra route
- **WHEN** configured, BYOK and refused callers send requests to the test adapter's route
- **THEN** refusal SHALL use the test adapter's format without reading the body, observations SHALL carry its label, and accepted calls SHALL run through the shared selector, runtime and option translation

#### Scenario: Test adapter is not shipped
- **WHEN** the plugin backend's package dependencies are listed
- **THEN** they SHALL NOT include the test adapter

### Requirement: Adapter contract documentation
Godoc SHALL describe the adapter contract: what an adapter supplies (a route handler over the shared runtime, a host-error writer, an optional discovery projection and an optional per-request option translator) and what the host provides (access, telemetry, selection, listing and per-attempt translation).

#### Scenario: Developer adds an adapter
- **WHEN** a developer reads the adapter contract godoc and the architecture walkthrough
- **THEN** they SHALL find each item an adapter supplies, each item the host provides, and the one route-table entry needed to register an adapter
