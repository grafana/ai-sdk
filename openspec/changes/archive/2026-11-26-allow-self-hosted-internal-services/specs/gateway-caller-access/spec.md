## MODIFIED Requirements

### Requirement: Explicit access mode
The Gateway SHALL require an explicit access mode, `cloud` or `self-hosted`, with no default. Startup SHALL fail when the mode is missing or unknown. Both modes SHALL accept the internal service allowlist.

#### Scenario: Missing access mode
- **WHEN** the Gateway starts without an access mode
- **THEN** startup SHALL fail with a secret-safe configuration error before serving requests

#### Scenario: Allowlist in self-hosted mode
- **WHEN** `self-hosted` mode is configured together with a valid internal service allowlist
- **THEN** startup SHALL succeed, and the allowlist SHALL apply as in `cloud` mode

### Requirement: Self-hosted mode access
In `self-hosted` mode, a request whose direct caller's service identity is on the internal allowlist SHALL receive configured-catalog access. Otherwise, a request whose identity actor is a user or service account SHALL receive configured-catalog access. Request-BYOK controls SHALL be refused with the existing reserved provider-options refusal. Every other request SHALL be refused with the fixed permission error before selection, catalog or provider work.

#### Scenario: Service account in self-hosted mode
- **WHEN** a Grafana service account calls in `self-hosted` mode
- **THEN** the request SHALL use configured accounts, the catalog and configured discovery

#### Scenario: Allowlisted plugin calls as itself
- **WHEN** a plugin whose signed service identity is on the allowlist calls in `self-hosted` mode with no user or service-account identity actor
- **THEN** the request SHALL use configured accounts, the catalog and configured discovery

#### Scenario: Allowlisted plugin calls for a user
- **WHEN** an allowlisted plugin calls in `self-hosted` mode on behalf of a user
- **THEN** the request SHALL use configured accounts, the catalog and configured discovery, with the user as the identity actor

#### Scenario: BYOK controls in self-hosted mode
- **WHEN** a self-hosted caller sends `providerOptions.gateway.byok`
- **THEN** the request SHALL be refused before catalog resolution and SHALL NOT use the supplied keys

#### Scenario: No identity actor in self-hosted mode
- **WHEN** the verified token has no user or service-account identity actor and its direct caller is not on the allowlist
- **THEN** the request SHALL be refused

### Requirement: Internal service allowlist configuration
The internal allowlist SHALL be explicit Gateway configuration of nonempty, unique, comma-separated service identity names, with surrounding whitespace removed from each entry, matched exactly and case-sensitively against the direct caller's service identity, in either access mode. An empty allowlist SHALL grant configured access to no caller through the allowlist. Invalid entries SHALL fail startup.

#### Scenario: Empty allowlist
- **WHEN** the allowlist is empty
- **THEN** no request SHALL receive configured access through the allowlist; in `cloud` mode users and service accounts SHALL retain BYOK access, and in `self-hosted` mode they SHALL retain configured access

#### Scenario: Near-match identity
- **WHEN** a direct caller's service identity differs from an allowlisted name only by case or surrounding whitespace
- **THEN** the caller SHALL NOT receive configured access

### Requirement: Verified caller attribution
Every served request SHALL carry a private verified caller: access class, route namespace, direct caller's service identity and identity actor's typed UID (`user:<uid>` or `service-account:<uid>`), each when present. Emitted caller-service attribution SHALL be the direct caller's service identity for allowlisted configured access and for catalog access, and omitted otherwise. UIDs follow caller identity attribution; other claims SHALL NOT enter logs, metrics, exports or provider requests.

#### Scenario: Non-configured call is observed
- **WHEN** a BYOK or relayed request, or a `self-hosted` request whose direct caller is not on the allowlist, on a namespace route is logged and exported
- **THEN** observations SHALL carry no caller-service value, even when the direct caller has a service identity

#### Scenario: Internal call on behalf of a user is observed
- **WHEN** an allowlisted service's call for a user is logged and exported, in either mode
- **THEN** observations SHALL carry the service identity, the namespace and the user's typed UID, and no other user claim SHALL appear in logs, metrics or exports

#### Scenario: Catalog call is observed
- **WHEN** an access policy's catalog call, alone or on behalf of a user, is logged and exported
- **THEN** observations SHALL carry its service identity, namespace and catalog name, the user's typed UID when it acts for one, and no other user claim
