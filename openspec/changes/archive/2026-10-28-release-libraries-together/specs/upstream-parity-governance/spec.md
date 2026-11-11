## MODIFIED Requirements

### Requirement: Publication-aware independent mergeability

Upgrade plans SHALL account for published Go dependencies before implementation. Coordinated producer/consumer changes MAY share an independently green candidate-source PR when required source parity/interop tests exercise them together through local first-party replacements and first-party requirements equal the version manifest. Required checks SHALL NOT rely on later unmerged source. The enclosing parity package SHALL still satisfy full behavioral acceptance.

#### Scenario: Workspace masks an unpublished dependency

- **WHEN** a coordinated source PR changes a producer and a consumer whose new behavior no published producer version contains
- **THEN** its required checks SHALL prove integrated candidate behavior and parity/interop independently of later PRs
- **AND** the next library release SHALL release both and update the consumer's requirement to the co-released producer version

#### Scenario: Parity upgrade has a failing required check

- **WHEN** a pinned-version upgrade fails a required parity, integration or candidate-source check
- **THEN** it SHALL correct that failure in the upgrade PR or wait rather than registering it as deferred nonblocking work

#### Scenario: Gateway publication at a coherent candidate revision

- **WHEN** a Gateway plugin package validates with same-revision SDK/provider/middleware source
- **THEN** it MAY publish only after source, requirement, boundary, plugin validation and protocol test host checks for that revision succeed
- **AND** its success SHALL NOT authorize any SDK/provider/middleware module tag without library release evidence

### Requirement: Source integration is not Go module release evidence

A green source PR SHALL NOT count as independently consumable Go-module release evidence. SDK/provider/middleware modules SHALL publish only through a validated library release pull request, and each published version SHALL pass post-publication proxy verification.

#### Scenario: Source integration is not Go module release evidence

- **WHEN** integrated candidate source passes but no library release pull request has validated the selected middleware module
- **THEN** source success SHALL NOT authorize its module publication

### Requirement: Gateway container release evidence

Gateway SHALL be a plugin-only supported release component. Plugin publication/deployment SHALL require successful same-revision multiarchitecture plugin validation and protocol test host suites, not Gateway compilation against published first-party versions.

#### Scenario: Gateway container release evidence

- **WHEN** Gateway passes same-revision multiarchitecture plugin validation and protocol test host suites but would fail against published first-party versions
- **THEN** that failure SHALL NOT alone prevent its plugin publication/deployment under the required same-revision gates
