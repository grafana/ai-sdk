## MODIFIED Requirements

### Requirement: Upstream parity baseline manifest
The repository SHALL define a checked-in upstream parity baseline manifest that records the verified upstream AI SDK target for parity-sensitive work. The manifest SHALL include the upstream repository URL, the coherent TypeScript package versions used by all parity consumers and reference tooling, the verification status, the verification commands, and known intentional deviations or accepted gaps. Directly consumed packages and transitive packages used as authoritative parity references SHALL be registered so compatibility and upgrade workflows can enforce one coherent package set. A pinned-version transition SHALL retain its selected target record with the change, including exact per-package source commits rather than assuming every package shares the manifest's upstream commit.

#### Scenario: Contributor finds the verified upstream target
- **WHEN** a contributor needs to know which upstream AI SDK version this repository is verified against
- **THEN** the upstream parity baseline manifest identifies the upstream repository and package versions used for verification

#### Scenario: Baseline records known gaps
- **WHEN** the repository intentionally diverges from upstream behavior or lacks coverage for an upstream surface
- **THEN** the upstream parity baseline manifest records the deviation or gap with enough context for reviewers to classify it as intentional

#### Scenario: Package source commits differ
- **WHEN** a selected coherent package set contains packages published from different upstream commits
- **THEN** the retained target record identifies each exact source commit
- **AND** semantic review uses the matching package source and tests
