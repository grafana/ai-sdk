## MODIFIED Requirements

### Requirement: Legacy ProviderWire surface is absent

The repository SHALL NOT publish, build, test, document as available, or claim compatibility for the tolerant legacy unversioned `gateway/providerwire` server or `providers/grafana` client. It SHALL NOT provide compatibility aliases, forwarding shims, copied codecs, or a legacy transport mode at another import path. Provider-domain JSON marshal and unmarshal behavior SHALL remain unchanged as local representation behavior, but provider structs and marshalers SHALL NOT define ProviderWire HTTP bytes, validation, or compatibility.

Any strict protocol introduced after retirement SHALL live under an explicit versioned namespace such as `ai-gateway/providerwire/v4`. Versioned strict artifacts SHALL use independent protocol DTOs or schemas, SHALL NOT import or restore legacy codecs, and SHALL NOT weaken the retirement of the exact unversioned package and client paths.

#### Scenario: Legacy production packages remain removed
- **WHEN** the repository's tracked Go packages and modules are inspected
- **THEN** no Go package SHALL have the exact import path `github.com/grafana/ai-sdk/gateway/providerwire`
- **AND** `github.com/grafana/ai-sdk/providers/grafana` SHALL NOT exist
- **AND** no production package SHALL import or re-export either retired path

#### Scenario: Versioned strict contract is independent
- **WHEN** artifacts are added below `ai-gateway/providerwire/v4`
- **THEN** they SHALL define only the explicit strict V4 contract or later strict V4 implementation
- **AND** they SHALL NOT expose the deleted tolerant handler, codecs, wire values, or compatibility mode

#### Scenario: Legacy-only verification remains removed
- **WHEN** repository test workspaces, conformance suites, tasks, and CI jobs are inspected
- **THEN** the legacy `test/interop` harness and Grafana provider-wire conformance SHALL NOT exist or be registered
- **AND** a separately named strict V4 contract workspace SHALL NOT count as restoration of either legacy harness
- **AND** provider-independent integration tests and non-Grafana provider conformance SHALL remain registered

#### Scenario: Provider JSON behavior remains local
- **WHEN** provider-domain values are marshaled or unmarshaled after retirement
- **THEN** their existing JSON behavior SHALL remain unchanged
- **AND** that behavior SHALL NOT define ProviderWire HTTP bytes, validation, or compatibility

#### Scenario: Legacy compatibility is not advertised
- **WHEN** user documentation and parity coverage are inspected
- **THEN** they SHALL NOT advertise the removed server, client, or automated legacy wire compatibility
- **AND** any strict V4 compatibility statement SHALL identify its explicit version and evidence boundary

#### Scenario: Transport-independent capabilities remain
- **WHEN** the repository is built and tested after strict V4 contract artifacts are added
- **THEN** provider-domain packages, concrete provider implementations, `gateway/catalog`, fallback, registry, middleware, UI-message SSE, retained TypeScript integration tests, and their applicable tests SHALL remain available
