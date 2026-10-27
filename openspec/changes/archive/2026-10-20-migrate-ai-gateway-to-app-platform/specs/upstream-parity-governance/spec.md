# Spec Delta

## MODIFIED Requirements

### Requirement: Publication-aware independent mergeability

Upgrade plans SHALL account for published Go dependencies before implementation. Coordinated producer/consumer changes MAY share an independently green candidate-source PR when required source parity/interop tests exercise them together and internal pins stay real, downloadable, replacement-free and merged in canonical main. Required checks SHALL NOT rely on later unmerged source. The enclosing parity package SHALL still satisfy full behavioral acceptance.

#### Scenario: Workspace masks an unpublished dependency
- **WHEN** a coordinated source PR passes only with candidate workspace copies of producer and consumer changes while existing published pins are older but merged
- **THEN** its required checks SHALL prove integrated candidate behavior and parity/interop independently of later PRs
- **AND** standalone compilation failure SHALL NOT alone block source merge or Gateway plugin publication after same-revision plugin validation, but SHALL block an affected independently consumable Go module's publication until resolved

#### Scenario: Parity upgrade has a failing required check
- **WHEN** a pinned-version upgrade fails a required parity, integration or candidate-source check
- **THEN** it SHALL correct that failure in the upgrade PR or wait rather than registering it as deferred nonblocking work

#### Scenario: Gateway publication at a coherent candidate revision
- **WHEN** a Gateway plugin package validates with candidate SDK/provider/middleware source while its declared older published module pins cannot compile standalone
- **THEN** it MAY publish only after source, ancestry, boundary, plugin validation and protocol test host checks for that revision succeed
- **AND** its success SHALL NOT authorize any SDK/provider/middleware module tag without independent standalone verification

### Requirement: Gateway container release evidence

Gateway SHALL be a plugin-only supported release component. Plugin publication/deployment SHALL require successful same-revision workspace-source multiarchitecture plugin validation and protocol test host suites, not standalone Gateway readiness against older published pins.

#### Scenario: Gateway container release evidence
- **WHEN** Gateway passes same-revision workspace-source multiarchitecture plugin validation and protocol test host suites but fails against older pins
- **THEN** standalone failure SHALL NOT alone prevent its plugin publication/deployment under the required same-revision gates
