## MODIFIED Requirements

### Requirement: Legacy tolerant ProviderWire surface is absent

The repository SHALL NOT publish, build, test, document as available, or claim compatibility for the tolerant legacy unversioned `gateway/providerwire` server or the retired tolerant `providers/grafana` implementation. It SHALL NOT provide compatibility aliases, forwarding shims, copied legacy codecs, or a legacy transport mode at another import path. The `providers/grafana` import path MAY be published only as an independently implemented strict ProviderWire V4 client whose request codecs, response codecs, error decoders, and SSE readers remain private. Provider-domain JSON marshal and unmarshal behavior SHALL remain unchanged as local representation behavior, but provider structs and marshalers SHALL NOT define ProviderWire HTTP bytes, validation, or compatibility.

Any reusable strict server protocol introduced after retirement SHALL live under an explicit versioned namespace such as `ai-gateway/providerwire/v4`. Versioned server artifacts and a strict client under `providers/grafana` SHALL use independent protocol DTOs or schemas, SHALL NOT import or restore legacy codecs, and SHALL NOT weaken the retirement of the exact unversioned server package or tolerant transport behavior.

#### Scenario: Legacy production packages remain removed
- **WHEN** the repository's tracked Go packages and modules are inspected
- **THEN** no Go package SHALL have the exact import path `github.com/grafana/ai-sdk/gateway/providerwire`
- **AND** any `github.com/grafana/ai-sdk/providers/grafana` module SHALL implement only the strict ProviderWire V4 client contract
- **AND** no production package SHALL import or re-export the retired server path, legacy codecs, or a tolerant compatibility mode

#### Scenario: Versioned strict contract is independent
- **WHEN** artifacts are added below `ai-gateway/providerwire/v4`
- **THEN** they SHALL define only the explicit strict V4 contract or later strict V4 implementation
- **AND** they SHALL NOT expose the deleted tolerant handler, codecs, wire values, or compatibility mode

#### Scenario: Legacy-only verification remains removed
- **WHEN** repository test workspaces, conformance suites, tasks, and CI jobs are inspected
- **THEN** the legacy `test/interop` harness and tolerant Grafana provider-wire conformance SHALL NOT exist or be registered
- **AND** separately named strict V4 contract, differential-client, and authenticated black-box evidence SHALL NOT count as restoration of either legacy harness
- **AND** provider-independent integration tests and non-Grafana provider conformance SHALL remain registered

#### Scenario: Provider JSON behavior remains local
- **WHEN** provider-domain values are marshaled or unmarshaled after retirement
- **THEN** their existing JSON behavior SHALL remain unchanged
- **AND** that behavior SHALL NOT define ProviderWire HTTP bytes, validation, or compatibility

#### Scenario: Legacy compatibility is not advertised
- **WHEN** user documentation and parity coverage are inspected
- **THEN** they SHALL NOT advertise the removed unversioned server, retired tolerant client behavior, or automated legacy wire compatibility
- **AND** documentation for a current Grafana client SHALL identify the strict V4 contract and SHALL NOT imply tolerant legacy compatibility
- **AND** any strict V4 compatibility statement SHALL identify its explicit version and evidence boundary

#### Scenario: Transport-independent capabilities remain
- **WHEN** the repository is built and tested after strict V4 contract artifacts are added
- **THEN** provider-domain packages, concrete provider implementations, `ai-gateway/catalog`, fallback, registry, middleware, UI-message SSE, retained TypeScript integration tests, and their applicable tests SHALL remain available
