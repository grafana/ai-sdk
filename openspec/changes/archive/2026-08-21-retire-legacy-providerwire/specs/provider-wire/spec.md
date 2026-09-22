## ADDED Requirements

### Requirement: Legacy ProviderWire surface is absent

The repository SHALL NOT publish, build, test, document as available, or claim compatibility for the tolerant legacy `gateway/providerwire` server or `providers/grafana` client. It SHALL NOT provide compatibility aliases, forwarding shims, copied codecs, or a legacy transport mode at another import path. Provider-domain JSON marshal and unmarshal behavior SHALL remain unchanged as local representation behavior, but provider structs and marshalers SHALL NOT define ProviderWire HTTP bytes, validation, or compatibility.

#### Scenario: Production packages are removed
- **WHEN** the repository's tracked Go packages and modules are inspected
- **THEN** `gateway/providerwire` and `providers/grafana` SHALL NOT exist
- **AND** no production package SHALL import or re-export either retired path

#### Scenario: Legacy-only verification is removed
- **WHEN** repository test workspaces, conformance suites, tasks, and CI jobs are inspected
- **THEN** the legacy `test/interop` harness and Grafana provider-wire conformance SHALL NOT exist or be registered
- **AND** provider-independent integration tests and non-Grafana provider conformance SHALL remain registered

#### Scenario: Provider JSON behavior remains local
- **WHEN** provider-domain values are marshaled or unmarshaled after retirement
- **THEN** their existing JSON behavior SHALL remain unchanged
- **AND** that behavior SHALL NOT define ProviderWire HTTP bytes, validation, or compatibility

#### Scenario: Legacy compatibility is not advertised
- **WHEN** user documentation and parity coverage are inspected
- **THEN** they SHALL NOT advertise the removed server, client, or automated legacy wire compatibility

#### Scenario: Transport-independent capabilities remain
- **WHEN** the repository is built and tested after retirement
- **THEN** provider-domain packages, concrete provider implementations, `gateway/catalog`, fallback, registry, middleware, UI-message SSE, retained TypeScript integration tests, and their applicable tests SHALL remain available

## REMOVED Requirements

### Requirement: Wire package location and scope
**Reason**: The tolerant `gateway/providerwire` package is retired before the strict ProviderWire V4 adapter is introduced.
**Migration**: Root/server consumers may pin `github.com/grafana/ai-sdk@v0.1.0-alpha.1`; Grafana-client source remains available at that repository tag or in Git history without an independently fetchable version claim.

### Requirement: Single endpoint with streaming header switch
**Reason**: The legacy `/language-model` endpoint and its tolerant request handling are removed with the package.
**Migration**: Keep legacy servers on root `v0.1.0-alpha.1` until the strict ProviderWire V4 server is available.

### Requirement: Header constants
**Reason**: The exported constants belong exclusively to the removed legacy transport.
**Migration**: Do not copy these constants into application code; pin the root legacy module until the strict protocol package defines its independent API.

### Requirement: Generate response is JSON GenerateResult
**Reason**: Direct serialization of provider-domain results is not response authority for the planned strict protocol.
**Migration**: Continue using root `v0.1.0-alpha.1` for the old response format; migrate only when strict response DTOs are available.

### Requirement: Stream response is text/event-stream of JSON StreamParts
**Reason**: Direct serialization of provider-domain stream parts is the retired legacy wire behavior.
**Migration**: Continue using root `v0.1.0-alpha.1` for the old stream format; the later strict runtime will define its own bounded SSE contract.

### Requirement: Error envelope for non-2xx generate responses
**Reason**: The legacy error codec can expose provider-domain error material and is not retained for the safe strict protocol.
**Migration**: Pin root/server deployments to `github.com/grafana/ai-sdk@v0.1.0-alpha.1`; Grafana-client source remains available at that repository tag or in Git history, and no independently versioned nested module is claimed.

### Requirement: SSE encoder/decoder helpers
**Reason**: These helpers encode and decode the retired provider-domain wire format.
**Migration**: Consumers that require them must pin root `v0.1.0-alpha.1`; no compatibility helper is retained.

### Requirement: Request/response envelope helpers
**Reason**: These helpers directly serialize provider-domain values and are removed with the tolerant codec.
**Migration**: Pin root `v0.1.0-alpha.1` rather than copying or depending on the removed codec.

### Requirement: Auth metadata is provider-side, not wire-side
**Reason**: The requirement scoped responsibilities between two packages that are both removed.
**Migration**: Authentication ownership for the future service will be specified by its strict service work package.

### Requirement: Wire package has no orchestration knowledge
**Reason**: The package whose dependency boundary this requirement defined no longer exists.
**Migration**: Root/server consumers may use root `v0.1.0-alpha.1`; future strict protocol boundaries will be specified independently.

### Requirement: Wire round-trip test suite
**Reason**: Round-trip tests for retired provider-domain JSON and SSE would preserve a compatibility claim the repository no longer makes.
**Migration**: Use tests from the root rollback version when maintaining a pinned deployment.

### Requirement: Provider wire excludes obsolete tool approval result stream part
**Reason**: Tool-approval behavior over the legacy transport is removed with that transport.
**Migration**: Legacy consumers must remain pinned; future strict tool capability packages will specify approval transport.

### Requirement: PartResponseMeta carries provider over the wire
**Reason**: Private provider attribution in the provider-domain wire is not retained and conflicts with the planned strict privacy model.
**Migration**: Pin the appropriate legacy module version if this field is required; future strict output will use canonical public identity.

### Requirement: Decoders accept upstream LanguageModelV4 request shapes
**Reason**: Tolerant decoding of upstream and legacy Go shapes is explicitly superseded by the planned strict registered-client dialect.
**Migration**: Pin the legacy modules for tolerant behavior; wait for the strict V4 request schema and mapper before upgrading.

### Requirement: Encoders emit upstream LanguageModelV4 request/response shapes
**Reason**: Provider-domain JSON methods will no longer control public protocol bytes.
**Migration**: Pin the legacy modules; the future strict client and server will use explicit private wire DTOs.

### Requirement: HTTP error envelope matches upstream gateway
**Reason**: The legacy envelope is removed and will not be used as the strict protocol's safe error authority.
**Migration**: Pin the legacy modules until strict error DTOs and schemas are available.

### Requirement: Bidirectional upstream-client conformance
**Reason**: The existing harness verifies the retired tolerant handler and cannot establish compatibility for the future strict implementation.
**Migration**: Use root `v0.1.0-alpha.1` for the historical server and Grafana-client source at that repository tag or in Git history; exact-pinned strict contract evidence will be introduced in the next work package.
