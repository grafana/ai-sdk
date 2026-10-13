## MODIFIED Requirements

### Requirement: Small native failure summaries

Candidate-local projection SHALL retain available native message, type, string-or-number code and native status.

#### Scenario: Small native failure summaries policy
- **WHEN** a candidate-local native failure is summarized
- **THEN** It SHALL follow only a single error chain and SHALL NOT assign an arbitrary aggregate member to a candidate. It SHALL NOT publish original bodies, headers, request data, arbitrary causes or diagnostic details. Native JSON decoding SHALL be standard Go shallow decoding with numeric precision preserved. Pure projection SHALL NOT extract credentials from requests or diagnostic trees. Runtime delivery SHALL redact exact configured API-key matches from summary message, type and string code under gateway-attempt-failure-evidence; other provider-originated values SHALL remain available independently of logger field policy. Inputs SHALL remain unchanged. Malformed, unavailable or oversized diagnostics under the integrator's existing source/read limit SHALL degrade optional fields without creating an error or a new quota.

#### Scenario: Original native failure survives cancellation
- **WHEN** a canceled decision retains a candidate-native SourceErr
- **THEN** projection SHALL use that source for its native summary while retaining canceled outcome without altering the returned context error

#### Scenario: Useful native message has an endpoint and cause
- **WHEN** an API error contains a safe native message plus URL/cause information
- **THEN** the message SHALL remain available without serializing the URL/cause or suppressing it merely because those fields exist

#### Scenario: Provider echoes a credential in a summary
- **WHEN** a provider-originated message, type or string code contains a credential value
- **THEN** runtime delivery SHALL redact exact configured API-key matches and SHALL preserve caller-owned BYOK values that do not match configured keys, without mutating source errors; account authorization and request isolation SHALL remain unchanged

#### Scenario: Numeric code exceeds float precision
- **WHEN** a native error supplies a large numeric code
- **THEN** projection SHALL retain its numeric value without float conversion

### Requirement: Dormant foundation and owned schema

The Gateway SHALL define and test a compact namespace schema independently of production service/handler activation, without a new stream reader or retry/cleanup owner, a runtime schema dependency or an Apache client import. The implementation SHALL remove the old Gateway collector, model wrappers and diagnostic allocation machinery. Root SDK observation SHALL remain reusable without Gateway dependencies. Minimum Go versions SHALL remain unchanged.

#### Scenario: Only the foundation is applied
- **WHEN** this change is applied without the activation change
- **THEN** existing runtime output SHALL remain unchanged while focused Go and strict TypeScript schema tests exercise the new projection
