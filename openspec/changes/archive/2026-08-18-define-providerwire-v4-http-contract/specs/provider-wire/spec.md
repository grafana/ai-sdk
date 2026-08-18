## MODIFIED Requirements

### Requirement: Wire package location and scope

The repository SHALL define a `gateway/providerwire/` Go package that owns the complete tolerant legacy JSON+HTTP/SSE transport for remote `provider.LanguageModel` calls. The package SHALL contain the route/header constants, request/response envelopes, SSE stream-part encoding/decoding, error-envelope helpers, and reusable legacy server handler. It MUST NOT contain protobuf, Connect, or other binary-format machinery. The former `provider/wire/` package SHALL remain deleted, and the repository MUST NOT provide aliases, compatibility re-exports, or a forwarding shim at its old import path. Moving the exported helpers remains an intentional source-breaking import-path change; canonical encoded bytes and protocol shapes SHALL remain unchanged, except for the previously specified final-line EOF correction.

The strict pinned V4 HTTP contract SHALL be owned by the sibling `gateway/providerwire/v4` contract artifacts. The legacy package SHALL remain available and behaviorally unchanged while that sibling contains no production decoder, handler, client, or DTO API.

#### Scenario: Package import path
- **WHEN** a Go file in this repository or in `providers/grafana/` imports the legacy wire helpers
- **THEN** it SHALL import `github.com/grafana/ai-sdk/gateway/providerwire`

#### Scenario: No protobuf machinery
- **WHEN** the `gateway/providerwire/` directory is inspected
- **THEN** it SHALL NOT contain `.proto` files, generated `.pb.go` files, `wirepb/`, `wirepbconnect/`, `buf.gen.yaml`, or any Connect-related artifacts

#### Scenario: Old package is deleted without compatibility
- **WHEN** the repository is inspected after the move
- **THEN** `provider/wire/` SHALL not exist and no package SHALL alias or re-export `gateway/providerwire` symbols from the old path

#### Scenario: Canonical legacy wire output remains stable
- **WHEN** existing request, response, error-envelope, or SSE values are encoded through `gateway/providerwire`
- **THEN** their encoded bytes and protocol shapes SHALL remain unchanged

#### Scenario: Strict contract has separate ownership
- **WHEN** the strict pinned V4 HTTP contract artifacts are inspected
- **THEN** they SHALL live under `gateway/providerwire/v4` and SHALL NOT redefine the legacy package's exported codecs as their normative schema

#### Scenario: No strict runtime exists in the contract phase
- **WHEN** the V4 contract phase is complete
- **THEN** the repository SHALL still have no production V4 decoder, handler, client, model adapter, or public wire DTO hierarchy
