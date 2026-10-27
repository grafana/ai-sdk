# Spec Delta

## MODIFIED Requirements

### Requirement: BYOK capture policy is independent of returned data
Host authorization, explicit credential sources and request isolation SHALL remain intact. Operator capture SHALL be independent of returned values. Gateway-produced summaries SHALL apply gateway-attempt-failure-evidence's configured provider/exporter key policy while retaining BYOK-only echoes and catalog-independent eligibility. BYOK attempts SHALL be captured request-locally in order, and their publication SHALL remain an explicit capability gap.

#### Scenario: BYOK privacy and attribution policy
- **WHEN** the host processes request-only credentials and observes execution
- **THEN** Host controls SHALL be consumed for account selection rather than forwarded wholesale to native inference.
- **AND** Logging SHALL use configured field-aware redaction; other exporters SHALL own their capture policies, and metric labels SHALL NOT contain credential payloads.
- **AND** The host SHALL NOT inventory request credentials. Only Gateway-produced native summary fields SHALL apply the configured provider/exporter key policy; source errors, opaque metadata, ordinary content and unrelated BYOK echoes SHALL remain unchanged. Platform-delegated access tokens and other inbound platform credentials SHALL remain excluded.
- **AND** The host SHALL NOT synthesize diagnostics from an internal credential store or another request's state.
- **AND** Supported provider/model identity, application content and native response values SHALL remain available under their existing contracts.
- **AND** Configured execution attribution SHALL remain independent of logical BYOK observation.
- **AND** Event-local native SSE summaries SHALL remain eligible without catalog-owned attribution. The execution runtime SHALL capture BYOK account attempts in order through the same request-scoped observer as configured fallbacks, but BYOK unary/setup summaries and attempt overviews SHALL NOT be published; #420 owns their publication with honest request identity, not a new collector.
- **AND** Independently registered SDK observers SHALL remain effective; original native metadata SHALL remain opaque and SHALL NOT establish configured provenance.
- **AND** Diagnostic messages SHALL use bounded approved capability names/schema paths and bounded indices/counts, never arbitrary rejected key names, values or serialized bodies.
- **AND** Detailed public attempt evidence SHALL remain owned by its separate contract; this capability SHALL NOT introduce a second metadata/error protocol.

#### Scenario: Provider echoes the rejected key
- **WHEN** a fake provider rejection includes the selected dummy API key in authentication-related error material
- **THEN** available native summaries SHALL retain that BYOK echo unless it exactly matches a configured provider/exporter key, without changing Gateway classification or account isolation
- **AND** logger sensitive-field policy and metadata-only exporter exclusions SHALL be tested independently from caller output

#### Scenario: Ordinary application text resembles a key
- **WHEN** supported prompt/output text contains a key-looking string unrelated to authentication material
- **THEN** the response SHALL preserve that content rather than apply token-pattern censorship

#### Scenario: BYOK attempts are captured but not published
- **WHEN** a BYOK request's first selected-provider account fails and the second succeeds
- **THEN** the request's capture SHALL hold a failed then a selected attempt, and neither the unary result nor the stream finish SHALL carry a `gateway.execution` overview
