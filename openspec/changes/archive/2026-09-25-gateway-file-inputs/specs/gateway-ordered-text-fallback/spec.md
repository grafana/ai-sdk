## ADDED Requirements

### Requirement: Empty message options retain text fallback eligibility
Otherwise eligible text requests SHALL remain eligible for fallback when ordinary message-level provider-option namespaces retained by the selected-backend policy contain only empty JSON objects. The guard SHALL assess semantic emptiness rather than namespace-map length, without removing or mutating the options it receives. A retained namespace with any member, including null, false, zero, empty strings, arrays, or nested objects, SHALL count as active and remain rejected. Namespaces removed by the backend policy do not make text fallback ineligible; invalid options and reserved host namespaces SHALL fail before that policy runs. File content, effectful history, tools, and all other existing route restrictions SHALL remain enforced.

#### Scenario: Authenticated text failover preserves empty namespaces
- **WHEN** an authenticated unary or streaming text request carries message options `{"anthropic":{}}` for an Anthropic fallback route and the primary fails under existing retry rules before commitment
- **THEN** the request SHALL retain its prior text fallback eligibility
- **AND** each attempted candidate SHALL receive the same empty namespace object without removal or promotion to call options

#### Scenario: Active values are not recursively treated as empty
- **WHEN** retained Anthropic message options contain `{"anthropic":{"cacheControl":false}}`, `{"anthropic":{"cacheControl":null}}`, or `{"anthropic":{"cacheControl":{}}}`
- **THEN** the fallback route guard SHALL reject the request before physical invocation rather than recursively treating the namespace as empty

#### Scenario: Irrelevant options do not block text fallback
- **WHEN** an authenticated text request carries active options only under a namespace outside every candidate's selected-backend policy
- **THEN** that namespace SHALL be removed before the guard, no candidate SHALL receive it, and ordinary text fallback SHALL remain eligible

#### Scenario: Empty options do not enable file fallback
- **WHEN** a request combines an empty retained message-option namespace with files or disallowed tool history
- **THEN** the existing route restriction SHALL fail safely before any physical candidate is invoked
