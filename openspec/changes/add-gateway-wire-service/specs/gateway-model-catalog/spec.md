## MODIFIED Requirements

### Requirement: Provider-wire server composition remains host-owned

The `gateway/catalog` package SHALL remain independent of `net/http`, `gateway/runtime`, and all provider-wire protocol adapters.

Existing hosts MAY continue composing `catalog.ModelResolver` with the legacy `gateway/providerwire.ModelResolver` through a host-owned adapter. That adapter SHALL pass `r.Context()` to catalog resolution, return the resolved language model for execution, and preserve canonical public identity for host-owned policy or logging. It SHALL translate only `catalog.ErrUnknownModel` into a non-retryable HTTP 404 `*provider.APICallError` with the catalog error as its cause; other catalog or registry failures SHALL pass through unchanged.

New gateway protocol adapters SHALL instead receive a shared `gateway/runtime` constructed with a call-aware resolver. The runtime SHALL provide a default adapter from `catalog.ModelResolver` for normalized calls without unsupported routing controls. That adapter SHALL pass the invocation context and requested public ID to `ResolveModel`, preserve requested and canonical public identity, classify `catalog.ErrUnknownModel` without an HTTP-aware resolver bridge, and invoke the returned model. A host needing request-specific routing MAY provide a call-aware resolver that inspects normalized gateway options before delegating to one or more catalogs. This new composition path MUST NOT make `gateway/catalog` depend on runtime types or change the legacy resolver contract.

#### Scenario: Catalog dependency boundary

- **WHEN** imports and public types in the `gateway/catalog` package are inspected
- **THEN** they SHALL NOT import or expose `net/http`, `gateway/runtime`, `gateway/providerwire`, or `gateway/providerwire/v4`

#### Scenario: Successful legacy provider-wire adaptation

- **WHEN** a valid legacy provider-wire request is adapted to a gateway catalog resolver
- **THEN** the host adapter SHALL resolve with the original request context, return `ResolvedModel.Model` to the HTTP execution boundary, and make `ResolvedModel.ID` available to host-owned policy or logging

#### Scenario: Unknown public model maps to legacy HTTP 404

- **WHEN** catalog resolution in a legacy host adapter returns an error matching `catalog.ErrUnknownModel`
- **THEN** the host adapter SHALL return a non-retryable HTTP 404 `*provider.APICallError` whose cause is the original catalog error

#### Scenario: Non-catalog failure retains provider-wire normalization

- **WHEN** a configured public route returns a registry or provider failure that does not match `catalog.ErrUnknownModel`
- **THEN** the legacy host adapter SHALL pass that error through unchanged rather than misclassifying it as an unknown public model

#### Scenario: Default runtime adapter resolves with call context

- **WHEN** a new protocol adapter invokes the runtime with a normalized call that needs no additional routing controls
- **THEN** the default adapter SHALL pass the invocation context and requested public ID to `catalog.ModelResolver.ResolveModel`

#### Scenario: Call-aware routing remains outside catalog

- **WHEN** a host needs provider order, model fallback, or other request-specific routing
- **THEN** its runtime resolver MAY inspect normalized gateway options before using the catalog, while `gateway/catalog` remains unaware of runtime call types

#### Scenario: Canonical identity reaches the runtime outcome

- **WHEN** a catalog alias resolves successfully
- **THEN** the runtime outcome SHALL retain both the requested alias and `ResolvedModel.ID` on success or any later invocation failure

#### Scenario: Runtime classifies unknown model centrally

- **WHEN** catalog resolution returns an error matching `catalog.ErrUnknownModel`
- **THEN** the runtime SHALL classify it as a gateway unknown-model failure without requiring a host HTTP adapter

#### Scenario: Existing host remains source compatible

- **WHEN** an existing host constructs `gateway/providerwire.NewHandler` with its current request-aware resolver
- **THEN** it SHALL compile and retain its existing resolution and error-mapping behavior

#### Scenario: New and legacy services coexist by deployment routing

- **WHEN** a host evaluates both handlers during migration
- **THEN** it SHALL mount them under distinct base-URL prefixes or hosts because both use the same `/language-model` relative route and request headers

## ADDED Requirements

### Requirement: Request policy remains outside the catalog

Authentication, tenancy, model visibility, provider-bound input policy, and request-specific routing SHALL remain outside `gateway/catalog`. Model-ID-only visibility MAY continue decorating `catalog.ModelResolver`. Policy that needs normalized call options, provider headers, or gateway controls SHALL run through the runtime call-policy seam before call-aware resolution. A policy MAY return gateway category sentinels so the runtime derives one classification without exposing its private cause.

#### Scenario: Tenant model visibility permits a model

- **WHEN** a catalog decorator reads host-authenticated tenant identity from context and permits the requested model
- **THEN** it SHALL delegate to the underlying catalog and preserve its canonical result

#### Scenario: Provider-bound input policy runs before catalog

- **WHEN** policy needs to inspect a downstream authorization header or gateway fallback option
- **THEN** it SHALL use runtime call policy before resolution rather than adding protocol or provider fields to `gateway/catalog`

#### Scenario: Policy denies a call

- **WHEN** call policy denies access and returns a forbidden category with a private cause
- **THEN** no catalog lookup SHALL run and the protocol adapter SHALL map the derived classification without exposing that cause
