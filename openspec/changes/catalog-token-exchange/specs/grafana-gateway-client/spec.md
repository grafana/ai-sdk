## ADDED Requirements

### Requirement: Per-call stack catalog selection

`WithCatalog` SHALL select one `ModelCatalog` for `ListModels`, `DoGenerate` and `DoStream` on a token-exchange client. The client SHALL append `/modelcatalogs/<name>` after the resolved route namespace and before the operation suffix, without altering namespace or exchange rules or ProviderWire shapes. Without a catalog selection, the client SHALL use the existing prefix.

#### Scenario: Discover and invoke one stack catalog on behalf of a user
- **WHEN** a token-exchange caller selects catalog `team.eu`, namespace `stacks-1` and a user exchange subject in its call context
- **THEN** the client SHALL use `GET <root>/namespaces/stacks-1/modelcatalogs/team.eu/config` for discovery and `POST <root>/namespaces/stacks-1/modelcatalogs/team.eu/language-model` for unary and streaming calls
- **AND** each request SHALL carry the access token exchanged for `stacks-1` and that subject, only in `X-Access-Token`

#### Scenario: The same catalog name is selected in two stacks
- **WHEN** calls on one provider select the same catalog name with different concrete route and exchange namespaces
- **THEN** each request SHALL go to its own namespace's catalog and send its own namespace's exchanged token

#### Scenario: Catalog selection does not override namespace resolution
- **WHEN** a call selects a catalog and omits a required call namespace or selects a namespace conflicting with the configured namespace
- **THEN** it SHALL be refused before token exchange or Gateway I/O

#### Scenario: Wildcard exchange
- **WHEN** a provider configured to exchange for `*` serves a call that selects a namespace and catalog
- **THEN** a service calling as itself SHALL route to that namespace's catalog with its wildcard token, and a call on behalf of a user SHALL fail before token exchange or Gateway I/O

#### Scenario: Selected catalog is unavailable
- **WHEN** a selected catalog route returns an error
- **THEN** the client SHALL return that error using its existing handling and SHALL NOT retry the call on the namespace route

#### Scenario: Catalog is selected with a direct credential
- **WHEN** a call on a service-account or caller-managed access-token provider uses `WithCatalog`
- **THEN** it SHALL fail before I/O rather than ignore the selection or replace the configured prefix

### Requirement: Catalog name validation

The catalog name SHALL match the stored resource's `metadata.name` lookup syntax: 1 to 253 ASCII letters, digits, hyphens, underscores, colons or dots, excluding the path segments `.` and `..`. The client SHALL preserve the selected name as given and reject an invalid selection before token exchange or Gateway I/O, without echoing the name.

#### Scenario: Catalog name outside namespace grammar
- **WHEN** a call selects the valid catalog name `Team_EU:v2.main`
- **THEN** the client SHALL preserve it as the single path segment after `/modelcatalogs/`

#### Scenario: Catalog name is explicitly empty or otherwise invalid
- **WHEN** a selected name is empty, too long, a dot-segment, or contains a character outside the lookup syntax
- **THEN** the call SHALL fail locally, not select a namespace route

## MODIFIED Requirements

### Requirement: Exact ProviderWire routes and protected headers

The client SHALL append exactly `/config` for discovery and `/language-model` for calls to the selected ProviderWire API prefix (namespace or catalog), without discarding an existing path prefix. Every model call SHALL use `POST`, JSON content type, the requested model ID, specification version `4`, exact streaming value `true` or `false`, and the matching JSON or SSE accept type.

#### Scenario: Header ownership applies to every authentication flow
- **WHEN** constructor or call-level authentication headers are supplied
- **THEN** For every authentication flow, `Authorization`, `X-Access-Token` and `X-Grafana-Id` SHALL be reserved case-insensitively. Supplying any reserved name in configured headers SHALL fail construction. Supplying any reserved name in call-level headers SHALL fail before request serialization and network I/O, including when its value is empty. Each constructor SHALL own its single credential header; configured and call headers SHALL NOT add a competing credential.

#### Scenario: Unary request is emitted
- **WHEN** `DoGenerate` is invoked for model `assistant`
- **THEN** the request SHALL be `POST <base-prefix>/language-model` with streaming `false`, model ID `assistant`, specification `4`, JSON content and accept types, and exactly one value for each client-owned header

#### Scenario: Streaming request is emitted
- **WHEN** `DoStream` is invoked
- **THEN** the request SHALL use the same method, route, model, and specification headers with streaming `true` and SSE accept type

#### Scenario: Header names collide
- **WHEN** configured or call-level headers attempt to set client-owned headers and are not rejected by the authentication reserved-header rule
- **THEN** the client-owned effective values SHALL win and no duplicate client-owned protocol value SHALL be emitted

#### Scenario: Cloud reserved headers are configured
- **WHEN** configured headers contain a reserved credential or identity header under any casing
- **THEN** construction SHALL fail without network I/O or echoing its value in the error

#### Scenario: Cloud reserved headers are supplied per call
- **WHEN** call-level headers contain a reserved credential or identity header under any casing
- **THEN** the call SHALL fail before serializing that value into request metadata or issuing an HTTP request

#### Scenario: Body-carried headers are supplied
- **WHEN** `CallOptions.Headers` contains a representable header entry accepted by the selected authentication flow
- **THEN** it SHALL participate in outer-header composition and remain present in the serialized `headers` body member for the server to accept or reject

### Requirement: Public authentication guide security and reference boundaries

The shared guide SHALL cover URL and credential selection, least-privilege service account roles, access policy scopes and namespace access, HTTPS, secure storage and rotation without exposing internal deployment names, host configuration or test-harness details. Exhaustive Go API reference SHALL remain in godoc.

#### Scenario: Public authentication guide security and reference boundaries
- **WHEN** a user reads credential provisioning instructions
- **THEN** the guide SHALL cover least privilege, HTTPS, storage and rotation without internal deployment/host/harness details, leaving exhaustive API reference in godoc

#### Scenario: A plugin backend uses a stored catalog on behalf of a user
- **WHEN** a user reads how to call a stored stack catalog
- **THEN** the guide SHALL distinguish a direct credential's catalog base URL from token exchange's API root, and show how to select the namespace, user and catalog for both model calls and discovery without replacing Grafana's access checks

### Requirement: Base URL shape per credential flow

For a platform-hosted Gateway, the service-account and access-token base URL SHALL be the complete ProviderWire prefix on the platform origin: `/apis/<group>/<version>/namespaces/<namespace>` for namespace routes or that prefix followed by `/modelcatalogs/<name>` for catalog routes. The token-exchange base URL SHALL be the Gateway API root, `/apis/<group>/<version>`, and construction SHALL refuse a base URL with a `namespaces` path segment. The client SHALL append `/namespaces/<namespace>` for each resolved route namespace and then the optional selected catalog prefix.

#### Scenario: Token-exchange request is emitted
- **WHEN** a token-exchange provider with base URL `https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1` invokes `DoGenerate` for resolved namespace `stacks-1` without a selected catalog
- **THEN** the request SHALL be `POST https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-1/language-model`, and discovery without a catalog SHALL use `.../namespaces/stacks-1/config`

#### Scenario: Catalog-prefixed base URL
- **WHEN** construction receives a valid Grafana base URL ending in `/namespaces/stacks-1/modelcatalogs/team`
- **THEN** the token-exchange constructor SHALL refuse it before I/O, and the two direct credential constructors SHALL continue to use the prefix as configured
