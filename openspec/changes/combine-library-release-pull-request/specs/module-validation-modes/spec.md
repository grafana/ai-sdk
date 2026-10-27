# Spec Delta

## ADDED Requirements

### Requirement: Workspace resolution of unpublished first-party versions

Root `go.work` and `go.gateway.work` SHALL include one non-published workspace module whose manifest replaces every published first-party library with its in-repository directory. Workspace files SHALL NOT contain `replace` directives. Candidate-source checks SHALL build when first-party requirements name versions that are not yet tagged. The module SHALL NOT be published, released or reference Gateway.

#### Scenario: Release pull request requires untagged versions

- **WHEN** a combined release pull request makes libraries and Gateway require co-released versions that are not yet tagged
- **THEN** root workspace builds and the Gateway plugin backend build resolve those versions to in-repository source with readonly manifests and no network lookup

#### Scenario: Workspace replacement added

- **WHEN** `go.work` or `go.gateway.work` contains a `replace` directive
- **THEN** the structural check fails

#### Scenario: Incomplete resolution module

- **WHEN** the workspace module omits a published first-party library or points one outside its in-repository directory
- **THEN** the structural check fails

## MODIFIED Requirements

### Requirement: Explicit Gateway candidate-source integration

The repository SHALL provide a separately selected Gateway workspace with local SDK/provider/middleware dependencies and the non-published workspace resolution module, excluding Gateway from root `go.work`. Required Gateway source build/test/lint/vet and cross-language commands SHALL select it explicitly; SDK-only commands SHALL select root `go.work` and keep their integration testserver Gateway-free. Required ProviderWire Go-client captures SHALL use candidate root/client source, not stale published pins.

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

Required source checks MAY activate candidate integration only together with a blocking workspace-source Gateway plugin-validation path for eligible main/tag publication. Boundary, parity, integration and merged-pin checks SHALL remain blocking source checks; all-module standalone compilation SHALL not block an otherwise valid source PR. On a combined library release pull request, the merged-pin check SHALL verify co-released versions against archives built from the candidate tree and every other first-party pin against canonical merged history. Gateway standalone compilation SHALL NOT become a plugin publication prerequisite.

#### Scenario: Source PR with incomplete standalone dependencies
- **WHEN** a source PR passes required candidate-source checks but cannot build against its older merged published pins
- **THEN** it SHALL remain mergeable, while publication SHALL require successful same-revision workspace-source plugin validation and protocol test host suites rather than standalone Gateway readiness

#### Scenario: Release pull request with co-released pins
- **WHEN** the merged-pin check runs on a combined library release pull request
- **THEN** co-released versions pass only if their candidate archives match the recorded checksums
- **AND** a first-party pin that is neither co-released nor tagged on canonical main fails the check

#### Scenario: Ordinary source PR names an untagged version
- **WHEN** an ordinary source PR requires a first-party version that is not tagged
- **THEN** the merged-pin check fails

### Requirement: Selected consumer regression evidence

Library readiness SHALL run relevant deterministic consumer regression scenarios
against the candidate library and its selected dependency graph, using published
prerequisites and archives of co-released prerequisites built from the candidate tree.
Workspace substitutions for prerequisites SHALL NOT supply release evidence.
Unrelated candidate modules SHALL NOT be required to compile.

#### Scenario: Older dependency compiles but lacks behavior

- **WHEN** a selected dependency omits runtime changes required through the candidate base
- **THEN** freshness validation fails even if compilation succeeds

#### Scenario: Selected provider consumer

- **WHEN** a provider release is validated
- **THEN** consumer scenarios use that candidate provider with published or co-released candidate prerequisites and no local replacement

#### Scenario: Root with unready downstream source

- **WHEN** root is releasable and provider source awaits a new root version
- **THEN** root consumer validation does not import the unready provider source
