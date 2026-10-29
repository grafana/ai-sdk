# Spec Delta

## MODIFIED Requirements

### Requirement: Fail-closed activation

Library release automation SHALL remain disabled until artifact-specific readiness,
required-check protection, current-candidate authorization and App prerequisites are
reviewed. Activation SHALL account for already-merged pending release pull requests.
Published tags SHALL remain immutable. Gateway application delivery SHALL be
activated independently and SHALL NOT gate library activation.

#### Scenario: Incomplete Gateway integration

- **WHEN** Gateway application delivery integration is incomplete
- **THEN** versioned Gateway publication remains disabled, while dev catalog publication of `main` builds follows the `gateway-plugin-publication` capability
- **AND** independently verified libraries can activate after their prerequisites and explicit approval
