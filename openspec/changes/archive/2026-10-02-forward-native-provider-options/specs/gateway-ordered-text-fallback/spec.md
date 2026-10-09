## MODIFIED Requirements

### Requirement: Empty message options retain text fallback eligibility
Otherwise eligible text requests SHALL remain eligible for fallback when ordinary message-level provider-option namespaces contain only empty JSON objects. The guard SHALL assess semantic emptiness rather than namespace-map length, without removing or mutating the options it receives. A namespace with any member, including null, false, zero, empty strings, arrays, or nested objects, SHALL count as active and remain rejected. All ordinary namespaces SHALL remain present for the guard; selected-backend namespace/field filtering SHALL NOT sanitize active options into fallback eligibility. Invalid options and unconsumed reserved host namespaces SHALL fail before model invocation. File content, effectful history, tools, call/part options, headers and all other existing route restrictions SHALL remain enforced. Broader mapped-capability fallback eligibility SHALL remain separately owned work.

#### Scenario: Authenticated text failover preserves empty namespaces
- **WHEN** an authenticated unary or streaming text request carries message options `{"anthropic":{}}` for an Anthropic fallback route and the primary fails under existing retry rules before commitment
- **THEN** the request SHALL retain its prior text fallback eligibility
- **AND** each attempted candidate SHALL receive the same empty namespace object without removal or promotion to call options

#### Scenario: Active values are not recursively treated as empty
- **WHEN** Anthropic message options contain `{"anthropic":{"cacheControl":false}}`, `{"anthropic":{"cacheControl":null}}`, or `{"anthropic":{"cacheControl":{}}}`
- **THEN** the fallback route guard SHALL reject the request before physical invocation rather than recursively treating the namespace as empty

#### Scenario: Irrelevant active options are not silently removed
- **WHEN** an authenticated text request carries active ordinary message options only under a namespace irrelevant to its candidates
- **THEN** that namespace SHALL remain in the mapped request and the existing fallback guard SHALL reject it before physical invocation
- **AND** this rejection SHALL NOT be presented as native direct-route support or a permanent mapped-fallback policy

#### Scenario: Empty options do not enable file fallback
- **WHEN** a request combines an empty message-option namespace with files or disallowed tool history
- **THEN** the existing route restriction SHALL fail safely before any physical candidate is invoked
