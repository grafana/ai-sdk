## REMOVED Requirements

### Requirement: Gateway privacy remains independent
**Reason**: Its scenario required the Gateway to reject `includeRawChunks`. The Gateway now passes the caller's flag to the adapter, so the requirement is restated below with that scenario replaced.
**Migration**: None. The privacy rules carry over unchanged in "Gateway privacy is independent of native transport observability".

## ADDED Requirements

### Requirement: Gateway privacy is independent of native transport observability

Native transport observability SHALL NOT enable Gateway raw output or add private backend request bodies, response bodies, headers, or unknown metadata to Gateway normalization. Existing public response projection and protected-header enforcement SHALL remain unchanged.

#### Scenario: Enriched native result passes through Gateway
- **WHEN** Gateway serves a provider result containing native transport metadata
- **THEN** its public unary or streaming output omits the private native request/response bodies and headers

#### Scenario: Gateway raw output is caller-requested
- **WHEN** an authenticated Gateway request omits IncludeRawChunks or sets it false
- **THEN** the selected adapter receives false and the Gateway stream contains no raw events, whatever native transport observability the adapter records
