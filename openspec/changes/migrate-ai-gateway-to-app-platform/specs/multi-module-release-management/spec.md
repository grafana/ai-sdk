# Spec Delta

## MODIFIED Requirements

### Requirement: Gateway application release

Gateway SHALL remain independently versioned with changelogs, release pull requests,
and `ai-gateway/vX.Y.Z` application tags. Its main and versioned plugin packages SHALL use the
workspace at the selected revision. Gateway SHALL NOT wait for library tags, repins,
or standalone Go compilation. Relevant linked workspace changes SHALL contribute
Gateway release intent using release-please mechanisms, without a competing version
calculator or manufactured dependency changes.

#### Scenario: Linked SDK change changes the Gateway image

- **WHEN** a relevant SDK, provider or middleware change affects workspace-built Gateway plugin behavior
- **THEN** Gateway can receive application release intent without requiring an ai-gateway path change

#### Scenario: Application release validation

- **WHEN** a Gateway release candidate is prepared
- **THEN** same-revision application tests, plugin builds, plugin validation and protocol test host suites precede any publication
- **AND** library standalone readiness cannot substitute for that evidence

### Requirement: Fail-closed activation

Release automation, including release PR preparation, SHALL remain disabled until
artifact-specific readiness, required-check protection, current-candidate
authorization, App prerequisites and Gateway release
attribution are reviewed. The activation change SHALL account for already-merged
pending release pull requests. Published tags SHALL remain immutable.

#### Scenario: Incomplete Gateway integration

- **WHEN** workspace plugin validation or linked release attribution is incomplete
- **THEN** Gateway release readiness fails and release PR preparation and
  automatic tag creation remain disabled
