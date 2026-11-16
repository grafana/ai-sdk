# Spec Delta

## MODIFIED Requirements

### Requirement: No Gateway-specific role
Access to the namespace routes SHALL NOT require a Gateway-specific Grafana role or route access check beyond platform authentication, plugin app access and namespace authorization. Gateway manifest roles SHALL grant permissions on Gateway kinds only, not on the namespace routes.

#### Scenario: Viewer calls with own keys in cloud mode
- **WHEN** an authenticated stack user with no Gateway-specific role sends a BYOK request in `cloud` mode
- **THEN** the request SHALL be served subject to BYOK validation

#### Scenario: Catalog roles do not gate the namespace routes
- **WHEN** a caller without `aigateway:catalogreader` or `aigateway:catalogwriter` calls a namespace route
- **THEN** the access decision SHALL be the same as for a caller holding them
