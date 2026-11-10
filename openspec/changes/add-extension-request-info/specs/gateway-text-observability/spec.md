# Spec Delta

## MODIFIED Requirements

### Requirement: Trusted bounded request observation context

The Gateway SHALL create one private immutable observation view per accepted HTTP request. It SHALL contain only a bounded Gateway-generated opaque request correlation ID and the normalized verified caller service, namespace and identity actor's typed UID from the authenticated caller context. Empty optional values SHALL be omitted.

#### Scenario: Authenticated request is enriched
- **WHEN** authentication stores a normalized caller
- **THEN** logical logs and Agent Observability metadata SHALL contain the same request correlation ID, caller service, namespace and caller identity under fixed keys (`caller_identity` in logs, `gateway.caller_identity` in Agent Observability metadata)
- **AND** the direct provider's call headers and provider options SHALL contain none of those values unless an independent future policy explicitly approves them

#### Scenario: Untrusted headers attempt enrichment
- **WHEN** an authenticated request supplies arbitrary request, region, application, user, or tenant headers
- **THEN** those header values SHALL NOT replace or extend the trusted observation view
- **AND** none SHALL become logical metadata or provider-bound options through this capability

#### Scenario: Optional deployment context is absent
- **WHEN** the verified caller has no attributable service, namespace or identity actor
- **THEN** the field SHALL be absent from logical telemetry rather than populated from request input or a placeholder

#### Scenario: Caller identity outside the bound
- **WHEN** the identity actor's typed UID exceeds the observation value bound or contains a non-printable character
- **THEN** the caller identity SHALL be absent from logical telemetry

#### Scenario: Existing clients require no change
- **WHEN** the registered Vercel client or WP7 Go client performs a text call
- **THEN** no new request field, request header, response field, or response header SHALL be required or returned for observability correlation
