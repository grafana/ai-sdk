## MODIFIED Requirements

### Requirement: Publication-aware independent mergeability
Upgrade plans SHALL account for published Go module dependencies before implementation. Coordinated producer and consumer changes MAY land in a single independently green candidate-source PR when required source parity/interop checks exercise the changed implementations together and internal published pins remain real, downloadable, replacement-free and merged into canonical main. An upgrade PR SHALL NOT rely on a later unmerged source change to pass its required checks. Source integration SHALL NOT count as independently consumable release evidence: before manually publishing a selected module, maintainers SHALL validate its standalone public-proxy, readonly, workspace-off build and tests; Gateway image publication and deployment SHALL require successful standalone artifact validation for the same revision. The enclosing parity work package SHALL additionally satisfy its full behavioral acceptance contract.

#### Scenario: Workspace masks an unpublished dependency
- **WHEN** a coordinated source PR passes only with candidate workspace copies of producer and consumer changes while existing published pins are older but merged
- **THEN** its required checks SHALL prove integrated candidate behavior and parity/interop independently of later PRs
- **AND** standalone compilation failure SHALL NOT alone block source merge but SHALL block an affected artifact's publication until resolved

#### Scenario: Parity upgrade has a failing required check
- **WHEN** a pinned-version upgrade fails a required parity, integration or candidate-source check
- **THEN** it SHALL correct that failure in the upgrade PR or wait rather than registering it as deferred nonblocking work
