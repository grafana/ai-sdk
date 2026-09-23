## ADDED Requirements

### Requirement: Empty message options retain text fallback eligibility
Otherwise eligible text requests SHALL remain eligible for fallback when supported ordinary message-level provider-option namespaces contain only empty JSON objects. The guard SHALL assess semantic emptiness rather than namespace-map length, without removing or mutating the preserved options. A namespace with any member, including null, false, zero, empty strings, arrays, or nested objects, SHALL count as active and remain rejected. Invalid options and reserved host namespaces SHALL NOT qualify for this exception. File content, effectful history, tools, and all other existing route restrictions SHALL remain enforced.

#### Scenario: Authenticated text failover preserves empty namespaces
- **WHEN** an authenticated unary or streaming text request carries message options `{"vendor":{}}` and the primary fails under existing retry rules before commitment
- **THEN** the request SHALL retain its prior text fallback eligibility
- **AND** each attempted candidate SHALL receive the same empty namespace object without removal or promotion to call options

#### Scenario: Active values are not recursively treated as empty
- **WHEN** message options contain `{"vendor":{"flag":false}}`, `{"vendor":{"value":null}}`, or `{"vendor":{"nested":{}}}`
- **THEN** the fallback route guard SHALL reject the request before physical invocation rather than recursively treating the namespace as empty

#### Scenario: Empty options do not enable file fallback
- **WHEN** a request combines ordinary empty message-option namespaces with files or disallowed tool history
- **THEN** the existing route restriction SHALL fail safely before any physical candidate is invoked
