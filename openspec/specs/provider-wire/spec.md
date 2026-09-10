# provider-wire Specification

## Purpose

Record the retirement boundary for the former remote `provider.LanguageModel` transport while preserving provider-domain JSON as local representation behavior only.

## Requirements

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
