## MODIFIED Requirements

### Requirement: Access decision metrics
The Gateway SHALL count route requests by bounded access decision: configured, BYOK, catalog or refused. Labels SHALL NOT contain caller identity, service identity, namespace or token values. Authentication outcomes owned by the platform SHALL NOT be counted as Gateway authentication metrics.

#### Scenario: Mixed callers
- **WHEN** internal, user, catalog route and refused callers send requests
- **THEN** the access-decision counter SHALL increase under its four fixed values only
