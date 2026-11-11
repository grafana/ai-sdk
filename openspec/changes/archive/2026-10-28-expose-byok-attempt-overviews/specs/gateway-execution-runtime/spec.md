# Spec Delta

## MODIFIED Requirements

### Requirement: Request-local attempt capture
Every selection SHALL get its own attempt capture, scoped to the call's context. Attempts observed after the capture is sealed SHALL be ignored. Configured calls SHALL publish the same execution overview as before. BYOK calls SHALL publish their captured attempts with request-account attribution. Selections with neither attribution SHALL NOT publish an overview.

#### Scenario: Concurrent calls are isolated
- **WHEN** two calls run concurrently on one runtime with different fallback outcomes
- **THEN** each call's capture SHALL contain only its own attempts

#### Scenario: BYOK attempts
- **WHEN** a BYOK call tries two accounts and the second is selected
- **THEN** the capture SHALL hold a failed then a selected attempt, and the response SHALL carry an overview with the requested selector, no canonical identity and those two native attempts in order

#### Scenario: Attempt after sealing
- **WHEN** an attempt is observed after the call's outcome was decided
- **THEN** it SHALL NOT appear in the overview

#### Scenario: Selection without attribution
- **WHEN** a selector returns a valid selection with neither configured nor request-account attribution and its model reports fallback attempts
- **THEN** the attempts SHALL be captured and sealed, and no overview SHALL appear in the response

#### Scenario: Test adapter sees BYOK attempts
- **WHEN** the test-only adapter and ProviderWire run the same BYOK fake-model scenarios
- **THEN** both SHALL report the same attempt view, including omitted canonical identity
