## MODIFIED Requirements

### Requirement: Provider-wire server composition remains host-owned

`gateway/catalog` SHALL remain independent of `net/http` and provider-wire protocol adapters.

Existing hosts MAY continue adapting `catalog.ModelResolver` to legacy `gateway/providerwire.ModelResolver` with host-owned HTTP/error behavior. The strict `gateway/providerwire/v4` handler SHALL instead accept `catalog.ModelResolver` directly, pass a context derived from the original request and the exact requested public model ID to `ResolveModel`, reject a nil returned model as an internal adapter defect, and invoke `ResolvedModel.Model` directly. This composition MUST NOT change the legacy resolver contract or make catalog depend on either provider-wire package.

#### Scenario: Catalog remains transport independent

- **WHEN** catalog imports and public types are inspected
- **THEN** they SHALL NOT expose `net/http`, legacy provider wire, or strict provider wire types

#### Scenario: Strict handler resolves exact model

- **WHEN** a valid strict request reaches model resolution
- **THEN** the handler SHALL call `ResolveModel` with the request-derived context and exact model header value

#### Scenario: Strict handler invokes resolved model

- **WHEN** resolution returns a non-nil `ResolvedModel.Model`
- **THEN** the strict handler SHALL call that model directly for the selected operation

#### Scenario: Catalog miss maps at strict adapter

- **WHEN** resolution returns an error matching `catalog.ErrUnknownModel`
- **THEN** the strict handler SHALL produce its private redacted 404 mapping with the requested public ID

#### Scenario: Legacy host remains source compatible

- **WHEN** an existing host constructs legacy `gateway/providerwire.NewHandler` with its request-aware resolver
- **THEN** compilation and existing resolution/error behavior SHALL remain unchanged

#### Scenario: Dual handlers use deployment routing

- **WHEN** a host evaluates both handlers
- **THEN** it SHALL mount them under distinct base URLs or hosts because both retain `/language-model`
