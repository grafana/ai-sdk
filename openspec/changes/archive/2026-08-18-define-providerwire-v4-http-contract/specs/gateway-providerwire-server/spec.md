## MODIFIED Requirements

### Requirement: Public complete provider-wire package and dependency boundary

The repository SHALL provide a public `github.com/grafana/ai-sdk/gateway/providerwire` package that owns the complete active legacy remote `provider.LanguageModel` protocol and reusable server execution surface. The package SHALL co-locate legacy route/header constants, JSON request/response codecs, SSE framing/readers/writers, error envelopes, and the `net/http` handler. It SHALL depend on `github.com/grafana/ai-sdk/provider` as the transport-agnostic in-process contract and MUST NOT import a router, auth library, host catalog, billing, IAM, deployment, or frontend orchestration package. The repository SHALL keep `github.com/grafana/ai-sdk/provider/wire` deleted and MUST NOT provide aliases, compatibility re-exports, or a forwarding shim at that path.

The sibling `gateway/providerwire/v4` capability SHALL define contract artifacts only during its contract phase. It SHALL NOT replace, wrap, or change the active legacy handler. Grafana SHALL continue to use the legacy server and client behavior by default until a later capability explicitly introduces and adopts a strict V4 runtime.

#### Scenario: Public legacy handler import
- **WHEN** a Go host imports `github.com/grafana/ai-sdk/gateway/providerwire`
- **THEN** it can construct the existing legacy `http.Handler` for provider language-model requests

#### Scenario: Canonical legacy codecs are co-located
- **WHEN** the legacy server decodes requests or encodes responses
- **THEN** it SHALL use the package's co-located exported codec helpers and provider runtime types rather than a separate wire package or gateway-specific DTOs

#### Scenario: Dependency graph remains one-way
- **WHEN** root and Grafana-provider imports are inspected
- **THEN** `gateway/providerwire` SHALL depend on `provider`, `providers/grafana` SHALL depend on both `provider` and `gateway/providerwire`, and `provider` SHALL NOT depend on gateway transport code

#### Scenario: Old package is absent
- **WHEN** the repository package tree is inspected after migration
- **THEN** `provider/wire` SHALL be absent and no compatibility import path or re-export SHALL remain

#### Scenario: Host dependencies stay outside
- **WHEN** imports and public types in `gateway/providerwire` are inspected
- **THEN** no Gorilla mux, authlib, JWKS, Grafana Assistant catalog, IAM, billing, route-prefix, or deployment type SHALL be present

#### Scenario: V4 contract artifacts do not alter runtime dispatch
- **WHEN** the V4 contract phase is complete
- **THEN** requests served through the public legacy handler SHALL follow the existing legacy codecs, validation, resolver, invocation, and response behavior

#### Scenario: Grafana remains on the rollback path
- **WHEN** Grafana constructs its provider without a later explicit strict-mode capability
- **THEN** it SHALL continue to use the legacy provider-wire transport with unchanged defaults
