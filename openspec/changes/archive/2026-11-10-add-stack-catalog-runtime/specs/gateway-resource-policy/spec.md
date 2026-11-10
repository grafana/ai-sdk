## ADDED Requirements

### Requirement: Stack-catalog bounds
Every stack-catalog request SHALL hold the stored catalog to the operator catalog's shape bounds: 1,024 public IDs including aliases, 128 aliases and 8 candidates per model, 128-byte public IDs, and 2,048 bytes per name, description, provider and model value. A call SHALL construct at most 8 provider models and SHALL retain no catalog, built model or key after it ends. Grafana's storage, not a Gateway setting, SHALL bound the stored object's size.

#### Scenario: Catalog above a shape bound
- **WHEN** a stored stack catalog has 1,025 public IDs including aliases
- **THEN** every call against it SHALL fail with the catalog configuration category, with zero provider requests

#### Scenario: No state across requests
- **WHEN** many calls run against many stack catalogs
- **THEN** the Gateway SHALL keep no per-catalog model, key or observer after each call ends

## MODIFIED Requirements

### Requirement: Documented limit ownership
The "Limits" section of `ai-gateway/README.md` SHALL list, for operators, each limit a request can meet and whether Grafana or the Gateway sets it. It SHALL describe current behavior only, without internal component names, removed limits or planned work.

#### Scenario: Ledger content
- **WHEN** an operator reads the Limits section
- **THEN** it SHALL list the Gateway's provider attempt bound, non-streamed response size, response-header timeout, redirect rejection, stream idle limit, and the operator and stack catalog limits
- **AND** it SHALL list Grafana's request duration, request size and concurrency, and the container memory limit
- **AND** it SHALL state that a request reaching Grafana's route timeout is recorded as canceled in Gateway logs and metrics, and that the termination grace period must exceed the route timeout
- **AND** it SHALL state that memory per request grows with the request and response size and JSON encoding, that total memory grows with concurrency, and recommend setting `GOMEMLIMIT` to about 85–90% of the container memory limit

### Requirement: Shared provider attempt bound
One fixed bound of 8 provider attempts per request SHALL limit operator-catalog chains, stack-catalog chains and BYOK accounts. A catalog model's primary plus fallbacks SHALL be at most 8, checked at operator catalog load and on every stack-catalog request. A BYOK request's selected-provider accounts SHALL be at most 8, checked at decoding. A composed routing chain SHALL be at most 8 candidates, checked before any provider request. The bound SHALL NOT be a setting.

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

#### Scenario: Stack-catalog chain above the bound
- **WHEN** a stored stack catalog has a model with a primary and 8 fallbacks
- **THEN** a call to any of its models SHALL fail with the catalog configuration category, with zero provider requests
