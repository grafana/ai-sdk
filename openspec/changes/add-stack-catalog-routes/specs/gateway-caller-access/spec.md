## ADDED Requirements

### Requirement: Catalog route access
On a catalog route, the Gateway SHALL grant `catalog` access to a verified caller whose token namespace covers the route namespace, when its identity actor is a user or service account, or when it is a service acting alone: its actor chain holds only services. It SHALL refuse every other caller with the fixed permission error. `access_mode` and the internal allowlist SHALL NOT apply, and Grafana's declared checks SHALL decide who may invoke.

#### Scenario: User or service account
- **WHEN** a user session or a service account token calls a catalog route in either access mode
- **THEN** the request SHALL get `catalog` access, with the identity actor as the private caller identity

#### Scenario: Access policy alone
- **WHEN** an access-policy token with no user calls a catalog route
- **THEN** the request SHALL get `catalog` access, attributed to the access policy's service identity

#### Scenario: Access policy acting for a user
- **WHEN** an access policy calls on behalf of a user
- **THEN** the request SHALL get `catalog` access, attributed to the service identity, with the user as the private caller identity

#### Scenario: Token for another namespace
- **WHEN** the token's namespace does not cover the route namespace
- **THEN** the request SHALL be refused with the permission document, without reading the body

#### Scenario: Anonymous viewer
- **WHEN** an anonymous session calls a catalog route, directly or relayed by a service
- **THEN** the request SHALL be refused

### Requirement: Catalog access selects only its parent catalog
`catalog` access SHALL select and list only the route's parent catalog. It SHALL NOT reach the operator catalog, operator provider keys, BYOK accounts or another catalog, and a namespace route SHALL never grant `catalog` access.

#### Scenario: Model only in the operator catalog
- **WHEN** a catalog route caller requests a model the operator catalog defines and the parent catalog does not
- **THEN** the call SHALL fail with the model not found document, with zero provider requests

#### Scenario: Catalog caller on a namespace route
- **WHEN** the same caller calls a namespace route
- **THEN** the access decision SHALL follow the access mode exactly as without catalog routes

## MODIFIED Requirements

### Requirement: No Gateway-specific role
Access to the namespace routes SHALL NOT require a Gateway-specific Grafana role or route access check beyond platform authentication, plugin app access and namespace authorization. Gateway manifest roles SHALL grant permissions on Gateway kinds and their catalog routes only, never on the namespace routes.

#### Scenario: Viewer calls with own keys in cloud mode
- **WHEN** an authenticated stack user with no Gateway-specific role sends a BYOK request in `cloud` mode
- **THEN** the request SHALL be served subject to BYOK validation

#### Scenario: Catalog roles do not gate the namespace routes
- **WHEN** a caller without `aigateway:catalogreader`, `aigateway:catalogwriter` or `aigateway:modelinvoker` calls a namespace route
- **THEN** the access decision SHALL be the same as for a caller holding them

### Requirement: Verified caller attribution
Every served request SHALL carry a private verified caller value with the access class, route namespace, the direct caller's service identity and the identity actor's typed UID (`user:<uid>` or `service-account:<uid>`), each when present. Emitted caller-service attribution SHALL be the direct caller's service identity for configured access in `cloud` mode and for catalog access, and SHALL be omitted otherwise. Identity-actor UIDs and other user claims SHALL NOT enter logs, metrics, exports or provider requests.

#### Scenario: Non-configured call is observed
- **WHEN** a BYOK, relayed or `self-hosted` request on a namespace route is logged and exported
- **THEN** observations SHALL carry no caller-service value, even when the direct caller has a service identity

#### Scenario: Internal call on behalf of a user is observed
- **WHEN** an allowlisted service's call for a user is logged and exported
- **THEN** observations SHALL carry the service identity and namespace, the private caller value SHALL hold the user's typed UID, and no user identifier SHALL appear in logs, metrics or exports

#### Scenario: Catalog call is observed
- **WHEN** an access policy's catalog call, alone or on behalf of a user, is logged and exported
- **THEN** observations SHALL carry its service identity, namespace and catalog name, and no user identifier
