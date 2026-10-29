# Spec Delta

## MODIFIED Requirements

### Requirement: Ordered direct text routes
The Gateway SHALL construct each public route from one required primary and zero or more ordered direct provider/backend candidates. Every invocation without routing controls SHALL begin at the primary and consider later candidates only in configured order. Only the request's own routing controls under gateway-request-routing MAY filter, reorder or extend candidates; no prior winner, other request value, health signal, or concurrent call SHALL reorder or skip candidates.

#### Scenario: Later call follows original order
- **WHEN** one call is served by a fallback candidate and a later call uses the same canonical route
- **THEN** the later call SHALL begin again with the configured primary

#### Scenario: Client attempts to select a backend
- **WHEN** a request carries a provider/fallback selection control other than `models`, `order` or `only`, or names a backend model ID or provider instance
- **THEN** existing strict validation or host policy SHALL reject it according to its owning contract and SHALL NOT alter candidate order

#### Scenario: Routing controls reorder one call
- **WHEN** a request carries `order`, `only` or `models` for a configured route
- **THEN** only that call's candidates SHALL follow gateway-request-routing, and later calls without controls SHALL begin again with the configured primary

#### Scenario: Mapped capability does not change ordering
- **WHEN** a supported function-tool, file, reasoning or ordinary-options request encounters eligible pre-commit failures in a three-candidate route
- **THEN** invoked candidates SHALL follow configured order with no capability-based reordering, silent primary-only execution or request mutation
