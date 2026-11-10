# Spec Delta

## MODIFIED Requirements

### Requirement: Verified caller attribution
Every served request SHALL carry a private verified caller value with the access class, route namespace, the direct caller's service identity and the identity actor's typed UID (`user:<uid>` or `service-account:<uid>`), each when present. Emitted caller-service attribution SHALL be the direct caller's service identity for configured access in `cloud` mode, and SHALL be omitted otherwise. UIDs follow caller identity attribution; other user claims SHALL NOT enter logs, metrics, exports or provider requests.

#### Scenario: Non-configured call is observed
- **WHEN** a BYOK, relayed or `self-hosted` request is logged and exported
- **THEN** observations SHALL carry no caller-service value, even when the direct caller has a service identity

#### Scenario: Internal call on behalf of a user is observed
- **WHEN** an allowlisted service's call for a user is logged and exported
- **THEN** observations SHALL carry the service identity, namespace and the user's typed UID, and no other user claim SHALL appear in logs, metrics or exports

## ADDED Requirements

### Requirement: Caller identity attribution
The identity actor's typed UID SHALL be available to every extension, operator and tenant, and SHALL be emitted as the caller identity in logical model logs and Agent Observability generation metadata for every selection source. It SHALL NOT become a metric label and SHALL NOT enter provider request headers, bodies or options.

#### Scenario: User's BYOK call
- **WHEN** a user calls with request BYOK
- **THEN** the call's logical log records and Agent Observability generation SHALL carry `user:<uid>`, and no metric label or provider request SHALL contain it

#### Scenario: Internal service without a user
- **WHEN** an allowlisted service calls for itself
- **THEN** no caller identity SHALL be emitted
