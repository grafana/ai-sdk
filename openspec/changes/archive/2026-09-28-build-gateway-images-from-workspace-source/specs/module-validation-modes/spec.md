## MODIFIED Requirements

### Requirement: Explicit Gateway candidate-source integration

The repository SHALL provide a separately selected Go workspace containing Gateway and its local SDK, provider and middleware source dependencies, without adding Gateway to root `go.work`. Required Gateway source build/test/lint/vet and cross-language commands SHALL explicitly select this workspace; SDK-only commands SHALL select root `go.work`, and the SDK-only integration testserver SHALL remain Gateway-free. Required ProviderWire Go-client captures SHALL use candidate root/client source rather than stale published pins. Gateway release/production **container** builds SHALL explicitly select that same checked-in Gateway workspace at the selected checkout revision with readonly manifests; standalone Go-module validation SHALL still use `GOWORK=off` and published dependencies. An ambient development workspace SHALL NOT determine image dependencies.

#### Scenario: Candidate-source Gateway integration
- **WHEN** coordinated source changes affect SDK and Gateway and required checks run
- **THEN** Go tests and cross-language binaries SHALL execute the candidate implementations, not merely report local paths from `go list`

#### Scenario: Isolated Gateway validation
- **WHEN** a Gateway production image builds
- **THEN** it SHALL resolve local SDK/provider/middleware source at the exact selected revision through explicit `go.gateway.work`, with readonly manifests and no out-of-repository substitutions
- **AND** an on-demand standalone Gateway module diagnostic SHALL resolve declared published versions with `GOWORK=off` and no local replacements

#### Scenario: Root default workspace
- **WHEN** Go selects root `go.work`
- **THEN** Gateway SHALL be absent from that workspace

### Requirement: Selectable published-module standalone validation

The repository SHALL retain one-module and all-module standalone commands using a public Go proxy, clean module cache, `GOWORK=off`, readonly manifests, dependency download/verification, build/test and no committed local replacements. Inventory SHALL use tracked nested `go.mod` roots and declared paths. Unknown or local-only example/test selection SHALL fail. These commands SHALL be callable on demand, with selected SDK/provider/middleware standalone validation required before independently consumable Go-module publication under #245/#21; selected Gateway standalone results SHALL NOT block Gateway image publication or deployment, and all-module standalone results SHALL NOT block ordinary source PRs. Gateway remains a container-only supported release component even if Go tooling discovers its release tag.

#### Scenario: One published module
- **WHEN** a tracked module is selected by its registered module path or root
- **THEN** standalone validation SHALL exercise only that module with the same checks used by the all-module command

#### Scenario: All published modules
- **WHEN** the existing all-module entry point runs
- **THEN** it SHALL continue validating every tracked module including Gateway, providers, middleware, the Grafana client, and the SDK root without treating Gateway as a supported external Go-module release

#### Scenario: Invalid or local-only selection
- **WHEN** a caller selects an unregistered module or a deliberately local-only example/test module
- **THEN** validation SHALL fail rather than silently skipping it

#### Scenario: SDK module release versus Gateway image
- **WHEN** a Gateway container passes workspace-source image checks but a candidate SDK/provider/middleware module fails standalone validation
- **THEN** Gateway publication MAY proceed without the dependency being repinned or tagged first
- **AND** the failing independent Go module SHALL NOT be published on image success alone

### Requirement: No premature source-gate switch

Required source checks MAY activate candidate integration only together with a blocking workspace-source Gateway production image-validation path for eligible main/tag publication. Boundary, parity, integration and merged-pin checks SHALL remain blocking source checks; all-module standalone compilation SHALL not block an otherwise valid source PR. Gateway standalone compilation SHALL NOT become an image publication prerequisite.

#### Scenario: Source PR with incomplete standalone dependencies
- **WHEN** a source PR passes required candidate-source checks but cannot build against its older merged published pins
- **THEN** it SHALL remain mergeable, while publication SHALL require successful same-revision workspace-source image validation and native smoke rather than standalone Gateway readiness
