# Spec Delta

## MODIFIED Requirements

### Requirement: Verified delegated identity
The Gateway SHALL serve a route request only when the plugin host contract delivers a delegated access token whose signature, expiry, audience and namespace were verified against the platform's signing keys. The audience SHALL name the plugin ID or its API group, and the token namespace SHALL cover the route namespace. The Gateway SHALL have no unverified mode, and SHALL start idle, serving no route request, without a platform signing-key source.

#### Scenario: Token for another audience or namespace
- **WHEN** a delegated token names a different audience or a namespace that does not cover the route namespace
- **THEN** the request SHALL be refused before selection, catalog or provider work

#### Scenario: No signing-key source
- **WHEN** the plugin environment provides no platform signing-key source
- **THEN** the Gateway SHALL start idle without reading its settings, catalog or provider credentials, even when they are missing or invalid
- **AND** every route request SHALL be rejected without selection, catalog or provider work
- **AND** the plugin SHALL log that it is idle because no platform signing keys are configured

#### Scenario: Insecure authentication skipping without a signing-key source
- **WHEN** the plugin environment provides no platform signing-key source and requests insecure authentication skipping
- **THEN** the Gateway SHALL fail startup without serving inference
