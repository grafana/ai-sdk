# gateway-caller-access Specification

## Purpose
Define how the Gateway turns the caller identity verified and delegated by the App Platform plugin host, together with the route namespace and the configured access mode, into configured-catalog or request-BYOK access, and which callers it refuses.

## Requirements

### Requirement: Verified delegated identity
The Gateway SHALL serve a route request only when the plugin host contract delivers a delegated access token whose signature, expiry, audience and namespace were verified against the platform's signing keys. The audience SHALL name the plugin ID or its API group, and the token namespace SHALL cover the route namespace. The Gateway SHALL refuse to run without a platform signing-key source and SHALL have no unverified mode.

#### Scenario: Token for another audience or namespace
- **WHEN** a delegated token names a different audience or a namespace that does not cover the route namespace
- **THEN** the request SHALL be refused before selection, catalog or provider work

#### Scenario: No signing-key source
- **WHEN** the plugin environment provides no platform signing-key source, including when insecure authentication skipping is requested
- **THEN** the Gateway SHALL fail startup without serving inference

### Requirement: Explicit access mode
The Gateway SHALL require an explicit access mode, `cloud` or `self-hosted`, with no default. Startup SHALL fail when the mode is missing or unknown. The internal service allowlist SHALL apply only in `cloud` mode, and configuring it in `self-hosted` mode SHALL fail startup.

#### Scenario: Missing access mode
- **WHEN** the Gateway starts without an access mode
- **THEN** startup SHALL fail with a secret-safe configuration error before serving requests

#### Scenario: Allowlist in self-hosted mode
- **WHEN** `self-hosted` mode is configured together with an internal service allowlist
- **THEN** startup SHALL fail before serving requests

### Requirement: Caller chain interpretation
The Gateway SHALL read the verified token's actor chain, ignoring the token's own top-level signer, which is the plugin host. The direct caller SHALL be the first actor, and its service identity SHALL be the one the platform signed for that hop. The identity actor SHALL be the innermost actor when it is a user or service account. Unsigned request data SHALL NOT contribute to either.

#### Scenario: Service calls on behalf of a user
- **WHEN** an internal service calls with an on-behalf-of token for a user, and the host delegates it to the plugin
- **THEN** the direct caller SHALL be that service and the identity actor SHALL be that user

#### Scenario: Caller is relayed through another service
- **WHEN** a service relays a token that an allowlisted service obtained
- **THEN** the direct caller SHALL be the relaying service, not the allowlisted one

### Requirement: Cloud mode access
In `cloud` mode, a request whose direct caller's service identity is on the internal allowlist SHALL receive configured-catalog access. Otherwise, a request whose identity actor is a user or service account SHALL receive request-BYOK access. Every other request SHALL be refused with the fixed permission error before selection, catalog or provider work.

#### Scenario: Allowlisted internal service
- **WHEN** the direct caller's service identity is on the allowlist, with or without an identity actor
- **THEN** the request SHALL use configured accounts, the catalog and configured discovery

#### Scenario: User or service account
- **WHEN** the direct caller is not allowlisted and the identity actor is a user or service account
- **THEN** the request SHALL use request-BYOK access and SHALL NOT reach configured accounts or the catalog

#### Scenario: Service without an allowlist entry or user
- **WHEN** the direct caller is not allowlisted and there is no user or service-account identity actor
- **THEN** the request SHALL be refused rather than granted BYOK or configured access

### Requirement: Self-hosted mode access
In `self-hosted` mode, a request whose identity actor is a user or service account SHALL receive configured-catalog access, and request-BYOK controls SHALL be refused with the existing reserved provider-options refusal. Every other request SHALL be refused with the fixed permission error before selection, catalog or provider work.

#### Scenario: Service account in self-hosted mode
- **WHEN** a Grafana service account calls in `self-hosted` mode
- **THEN** the request SHALL use configured accounts, the catalog and configured discovery

#### Scenario: BYOK controls in self-hosted mode
- **WHEN** a self-hosted caller sends `providerOptions.gateway.byok`
- **THEN** the request SHALL be refused before catalog resolution and SHALL NOT use the supplied keys

#### Scenario: No identity actor in self-hosted mode
- **WHEN** the verified token has no user or service-account identity actor
- **THEN** the request SHALL be refused

### Requirement: Internal service allowlist configuration
The internal allowlist SHALL be explicit Gateway configuration of nonempty, unique, comma-separated service identity names, with surrounding whitespace removed from each entry, matched exactly and case-sensitively against the direct caller's service identity. An empty allowlist SHALL grant configured access to no caller. Invalid entries SHALL fail startup.

#### Scenario: Empty allowlist
- **WHEN** the allowlist is empty in `cloud` mode
- **THEN** no request SHALL receive configured access, and users and service accounts SHALL retain BYOK access

#### Scenario: Near-match identity
- **WHEN** a direct caller's service identity differs from an allowlisted name only by case or surrounding whitespace
- **THEN** the caller SHALL NOT receive configured access

### Requirement: Request content cannot change access
Request parameters, headers, provider options, model selectors and supplied provider keys SHALL NOT change the derived account access, caller attribution or tenant context. The existing configured and request-BYOK selection refusals SHALL apply unchanged to the access derived from identity and mode.

#### Scenario: Request tries to widen access
- **WHEN** a BYOK caller selects a configured alias, or a configured caller sends BYOK controls
- **THEN** the existing selection refusal for the derived access SHALL apply before catalog or provider I/O

### Requirement: Route namespace tenant context
The tenant context for attribution SHALL be the route namespace the platform authorized. The Gateway SHALL NOT derive a tenant from request headers or bodies and SHALL accept any namespace the platform authorizes, including single-tenant namespaces.

#### Scenario: Wildcard internal token
- **WHEN** an internal caller's delegated token covers all namespaces and the route names one namespace
- **THEN** attribution SHALL use the route namespace

#### Scenario: Header names another tenant
- **WHEN** a request supplies an `X-Scope-OrgID` or another tenant header naming a different namespace
- **THEN** the header SHALL be ignored for access and attribution

### Requirement: Verified caller attribution
Every served request SHALL carry a private verified caller value with the access class, route namespace, the direct caller's service identity and the identity actor's typed UID (`user:<uid>` or `service-account:<uid>`), each when present. Emitted caller-service attribution SHALL be the direct caller's service identity for configured access in `cloud` mode, and SHALL be omitted otherwise. Identity-actor UIDs and other user claims SHALL NOT enter logs, metrics, exports or provider requests.

#### Scenario: Non-configured call is observed
- **WHEN** a BYOK, relayed or `self-hosted` request is logged and exported
- **THEN** observations SHALL carry no caller-service value, even when the direct caller has a service identity

#### Scenario: Internal call on behalf of a user is observed
- **WHEN** an allowlisted service's call for a user is logged and exported
- **THEN** observations SHALL carry the service identity and namespace, the private caller value SHALL hold the user's typed UID, and no user identifier SHALL appear in logs, metrics or exports

### Requirement: No Gateway-specific role
Access SHALL NOT require a Gateway-specific Grafana role or route access check beyond platform authentication, plugin app access and namespace authorization.

#### Scenario: Viewer calls with own keys in cloud mode
- **WHEN** an authenticated stack user with no Gateway-specific role sends a BYOK request in `cloud` mode
- **THEN** the request SHALL be served subject to BYOK validation
