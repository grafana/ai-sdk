# Spec Delta

## MODIFIED Requirements

### Requirement: Explicit Gateway candidate-source integration

The repository SHALL provide a separately selected Gateway workspace with local SDK/provider/middleware dependencies, excluding Gateway from root `go.work`. Required Gateway source build/test/lint/vet and cross-language commands SHALL select it explicitly; SDK-only commands SHALL select root `go.work` and keep their integration testserver Gateway-free. Required ProviderWire Go-client captures SHALL use candidate root/client source, not stale published pins.

#### Scenario: Candidate-source Gateway integration
- **WHEN** coordinated source changes affect SDK and Gateway and required checks run
- **THEN** Go tests and cross-language binaries SHALL execute the candidate implementations, not merely report local paths from `go list`

#### Scenario: Isolated Gateway validation
- **WHEN** a Gateway plugin backend builds
- **THEN** it SHALL resolve local SDK/provider/middleware source at the exact selected revision through explicit `go.gateway.work`, with readonly manifests and no out-of-repository substitutions
- **AND** an on-demand standalone Gateway module diagnostic SHALL resolve declared published versions with `GOWORK=off` and no local replacements

#### Scenario: Root default workspace
- **WHEN** Go selects root `go.work`
- **THEN** Gateway SHALL be absent from that workspace

### Requirement: No premature source-gate switch

Required source checks MAY activate candidate integration only together with a blocking workspace-source Gateway plugin-validation path for eligible main/tag publication. Boundary, parity, integration and merged-pin checks SHALL remain blocking source checks; all-module standalone compilation SHALL not block an otherwise valid source PR. Gateway standalone compilation SHALL NOT become a plugin publication prerequisite.

#### Scenario: Source PR with incomplete standalone dependencies
- **WHEN** a source PR passes required candidate-source checks but cannot build against its older merged published pins
- **THEN** it SHALL remain mergeable, while publication SHALL require successful same-revision workspace-source plugin validation and protocol test host suites rather than standalone Gateway readiness

### Requirement: Selectable published-module standalone validation

The repository SHALL retain one-module/all-module standalone commands using public Go proxy, clean cache, `GOWORK=off`, readonly manifests, download/verification, build/test and no committed local replacements. Inventory SHALL use tracked nested `go.mod` roots and declared paths; unknown/local-only example/test selection SHALL fail. These commands SHALL remain callable on demand.

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
- **WHEN** a Gateway plugin passes workspace-source plugin checks but a candidate SDK/provider/middleware module fails standalone validation
- **THEN** Gateway publication MAY proceed without the dependency being repinned or tagged first
- **AND** the failing independent Go module SHALL NOT be published on plugin success alone

### Requirement: Standalone validation publication boundary

Selected SDK/provider/middleware standalone validation SHALL be required before independently consumable Go-module publication under #245/#21. Selected Gateway standalone results SHALL NOT block plugin publication/deployment; all-module results SHALL NOT block ordinary source PRs. Gateway SHALL remain a plugin-only supported release even if Go tooling discovers its tag.

#### Scenario: Standalone validation publication boundary
- **WHEN** Go tooling discovers a Gateway release tag but standalone compilation fails
- **THEN** Gateway SHALL remain plugin-only, with plugin/deployment gating independent of standalone results; no independent SDK/provider/middleware publication SHALL bypass its own standalone check

## ADDED Requirements

### Requirement: Explicit workspace for Gateway plugin backend dependencies
Gateway release and production plugin backend builds SHALL explicitly select the checked-in Gateway workspace at the selected checkout revision with readonly manifests. Ambient development workspaces SHALL NOT determine backend dependencies. Standalone Go-module validation SHALL still use `GOWORK=off` and published dependencies.

#### Scenario: Explicit workspace for Gateway plugin backend dependencies
- **WHEN** a Gateway production plugin backend builds with a different ambient development workspace
- **THEN** its dependencies SHALL still come from the explicit checked-in Gateway workspace at the selected revision

## REMOVED Requirements

### Requirement: Explicit workspace for Gateway container dependencies
**Reason**: The Gateway container image is removed.
**Migration**: Use "Explicit workspace for Gateway plugin backend dependencies".
