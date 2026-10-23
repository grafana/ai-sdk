# Spec Delta

## ADDED Requirements

### Requirement: Explicit staged library activation

Library automation SHALL require explicit activation on canonical main pushes and verified protection settings. Root bootstrap SHALL select only the retained initial root candidate. Library mode SHALL exclude Gateway and preserve independent component outcomes. An absent or unknown mode SHALL prevent writes. Gateway delivery SHALL NOT gate library activation.

#### Scenario: Root bootstrap

- **WHEN** root mode is selected
- **THEN** only the retained bootstrap root candidate can publish
- **AND** no additional release PR is prepared

#### Scenario: Library progression

- **WHEN** library mode is selected and one component remains blocked
- **THEN** other independently valid libraries can publish and prepare releases
- **AND** Gateway receives no writes

#### Scenario: Missing protections

- **WHEN** strict source/readiness checks or immutable tag protections cannot be verified
- **THEN** activation refuses all release writes

## MODIFIED Requirements

### Requirement: Fail-closed activation

Library release automation SHALL remain disabled until artifact-specific readiness,
required-check protection, current-candidate authorization and App prerequisites are
reviewed. Activation SHALL account for already-merged pending release pull requests.
Published tags SHALL remain immutable. Gateway application delivery SHALL be
activated independently and SHALL NOT gate library activation.

#### Scenario: Incomplete Gateway integration

- **WHEN** Gateway application delivery integration is incomplete
- **THEN** Gateway publication remains disabled
- **AND** independently verified libraries can activate after their prerequisites and explicit approval
