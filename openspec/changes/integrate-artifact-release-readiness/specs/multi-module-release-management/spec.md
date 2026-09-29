## ADDED Requirements

### Requirement: Artifact-specific release readiness

Ordinary source pull requests SHALL validate coordinated candidate source and merged
internal pins without requiring every module to compile standalone. Library release
pull requests SHALL separately validate the selected module and actual dependency
closure against canonical tagged prerequisites with readonly standalone checks and a
fresh public-proxy cache. Prerequisite owned source SHALL cover the current candidate
base; old dependency compilation alone SHALL NOT establish readiness.

#### Scenario: Root release while downstream source waits

- **WHEN** root is ready and a downstream provider needs an unreleased prerequisite
- **THEN** the root release check validates root without requiring the unrelated provider to pass standalone

#### Scenario: Stale prerequisite behavior

- **WHEN** a pinned library prerequisite omits owned runtime source changes in the current base
- **THEN** the dependent library release fails readiness even if it compiles

#### Scenario: Verified release identity and current candidate

- **WHEN** a release manifest version changes
- **THEN** readiness verifies the dedicated App, canonical repository, generated branch, single component, generated-file footprint and exact current-base merge candidate
- **AND** a mutable label alone cannot authorize publication

#### Scenario: Library dependency progression

- **WHEN** root or a provider library publishes
- **THEN** Renovate advances library requirements in follow-up source pull requests
- **AND** root and intermediate dependency updates do not require unrelated downstream modules to pass standalone

### Requirement: Gateway application release

Gateway SHALL remain independently versioned with changelogs, release pull requests,
and `ai-gateway/vX.Y.Z` application tags. Its main and versioned images SHALL use the
workspace at the selected revision. Gateway SHALL NOT wait for library tags, repins,
or standalone Go compilation. Relevant linked workspace changes SHALL contribute
Gateway release intent using release-please mechanisms, without a competing version
calculator or manufactured dependency changes.

#### Scenario: Linked SDK change changes the Gateway image

- **WHEN** a relevant SDK, provider or middleware change affects workspace-built Gateway behavior
- **THEN** Gateway can receive application release intent without requiring an ai-gateway path change

#### Scenario: Application release validation

- **WHEN** a Gateway release candidate is prepared
- **THEN** same-revision application tests, image builds and runtime smoke validation precede publication
- **AND** library standalone readiness cannot substitute for that evidence

### Requirement: Fail-closed activation

Publication SHALL remain disabled until artifact-specific readiness, required-check
protection, current-candidate authorization, App prerequisites and Gateway release
attribution are reviewed. The activation change SHALL account for already-merged
pending release pull requests. Published tags SHALL remain immutable.

#### Scenario: Incomplete Gateway integration

- **WHEN** workspace image validation or linked release attribution is incomplete
- **THEN** Gateway release readiness fails and automatic tag creation remains disabled
