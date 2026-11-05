# Spec Delta

## MODIFIED Requirements

### Requirement: Shared provider attempt bound
One fixed bound of 8 provider attempts per request SHALL limit both configured chains and BYOK accounts. A configured model's primary plus fallbacks SHALL be at most 8, checked at configuration load. A BYOK request's selected-provider accounts SHALL be at most 8, checked when the request is decoded. A request's composed routing chain under gateway-request-routing SHALL be at most 8 candidates, checked before any provider request. The bound SHALL NOT be an operator setting.

#### Scenario: Configured chain at and above the bound
- **WHEN** a configured model has a primary and 7 fallbacks, or a primary and 8 fallbacks
- **THEN** the first SHALL load and the second SHALL fail configuration loading with the existing secret-safe cardinality error

#### Scenario: BYOK accounts at and above the bound
- **WHEN** the selected provider's BYOK array has 8 accounts, or 9 accounts
- **THEN** the first SHALL be accepted and the second SHALL receive the fixed BYOK credentials 400 with zero native calls

#### Scenario: Unused BYOK provider is not counted
- **WHEN** an unused provider's BYOK array has 9 or more well-formed accounts
- **THEN** the request SHALL be accepted, and those accounts SHALL NOT count toward the bound

#### Scenario: Routed chain at and above the bound
- **WHEN** routing controls compose a chain of 8 candidates, or of 9 candidates
- **THEN** the first SHALL run normally and the second SHALL fail with the fixed routing attempt-bound document, with zero native calls and no truncation
