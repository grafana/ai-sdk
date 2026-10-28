# Spec Delta

## MODIFIED Requirements

### Requirement: Request-local attempt capture
Every selection SHALL get its own attempt capture, scoped to the call's context. Attempts observed after the capture is sealed SHALL be ignored. The runtime SHALL record the selected output's served model identity, as defined by gateway-execution-overview, including markers observed after sealing, without changing the sealed attempts list. Configured calls SHALL publish the same attempts in the same order as before, differing only by `servedModelId` when one was recorded. BYOK calls SHALL publish their captured attempts with request-account attribution. Selections with neither attribution SHALL NOT publish an overview.

#### Scenario: Concurrent calls are isolated
- **WHEN** two calls run concurrently on one runtime with different fallback outcomes
- **THEN** each call's capture SHALL contain only its own attempts

#### Scenario: BYOK attempts
- **WHEN** a BYOK call tries two accounts and the second is selected
- **THEN** the capture SHALL hold a failed then a selected attempt, and the response SHALL carry an overview with the requested selector, no canonical identity and those two native attempts in order

#### Scenario: Attempt after sealing
- **WHEN** an attempt is observed after the call's outcome was decided
- **THEN** it SHALL NOT appear in the overview

#### Scenario: Served model after sealing
- **WHEN** a selected stream emits an `anthropic.fallback` marker after its first part sealed the capture
- **THEN** the overview SHALL carry that marker's served model on the selected attempt, and the attempts list SHALL equal the sealed list

#### Scenario: Selection without attribution
- **WHEN** a selector returns a valid selection with neither configured nor request-account attribution and its model reports fallback attempts
- **THEN** the attempts SHALL be captured and sealed, and no overview SHALL appear in the response

#### Scenario: Test adapter sees BYOK attempts
- **WHEN** the test-only adapter and ProviderWire run the same BYOK fake-model scenarios
- **THEN** both SHALL report the same attempt view, including omitted canonical identity

#### Scenario: Test adapter sees the served model
- **WHEN** the test-only adapter and ProviderWire run the same configured fake-model scenario whose output carries an `anthropic.fallback` marker
- **THEN** both SHALL report the same attempt view, including `servedModelId`
