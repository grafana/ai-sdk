## ADDED Requirements

### Requirement: Transport composition remains host-owned
The `gateway/catalog` package SHALL remain independent of `net/http` and any concrete transport adapter. A host-owned adapter MAY pass request context into catalog resolution, execute the returned model, preserve canonical public identity for policy or logging, and translate `catalog.ErrUnknownModel` at its own protocol boundary. The catalog SHALL NOT define HTTP status codes or protocol error envelopes.

#### Scenario: Catalog dependency boundary
- **WHEN** imports and public types in the `gateway/catalog` package are inspected
- **THEN** they SHALL NOT import or expose `net/http` or a concrete transport package

#### Scenario: Host adapts successful resolution
- **WHEN** a host resolves a public model through the catalog
- **THEN** it MAY pass `ResolvedModel.Model` to its execution boundary and retain `ResolvedModel.ID` as canonical public identity

#### Scenario: Host maps unknown models
- **WHEN** catalog resolution returns an error matching `catalog.ErrUnknownModel`
- **THEN** the host MAY map that error to its protocol-specific not-found response without the catalog owning that protocol

## MODIFIED Requirements

### Requirement: Catalog remains separate from registry and transport policy
The gateway catalog SHALL treat public IDs as opaque strings and SHALL NOT require registry separator syntax. This capability SHALL NOT change `registry.Provider`, `ProviderRegistry`, `CustomProvider`, provider call options, provider behavior, or transport payloads.

#### Scenario: Public ID contains no provider separator
- **WHEN** a static or registry-backed catalog registers a flat public ID
- **THEN** it SHALL resolve that ID without requiring `provider:model` syntax

#### Scenario: Existing registry is used directly
- **WHEN** an existing consumer calls `ProviderRegistry.LanguageModel` without using a gateway catalog
- **THEN** its composite-ID resolution and middleware behavior SHALL remain unchanged

#### Scenario: Catalog resolves a model
- **WHEN** a catalog returns an existing `provider.LanguageModel`
- **THEN** it SHALL NOT alter provider call options, provider requests, stream parts, UI chunks, or SSE framing

## REMOVED Requirements

### Requirement: Provider-wire server composition remains host-owned
**Reason**: The requirement is coupled to the retired `gateway/providerwire` handler and its error normalization.
**Migration**: Use the transport-neutral host-composition requirement; the future strict adapter will define its own catalog boundary.
