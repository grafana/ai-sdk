## ADDED Requirements

### Requirement: OpenAI Responses shared bounded transport
OpenAI Responses models SHALL use the shared model HTTP client and inherit its redirect rejection, fixed 10-second response-header timeout and fixed 16 MiB per-response decompressed byte bound under gateway-resource-policy. No provider-specific transport setting SHALL exist.

#### Scenario: OpenAI Responses shared bounded transport
- **WHEN** an OpenAI backend redirects, stalls its response headers or exceeds the response byte bound
- **THEN** the shared client SHALL enforce the same bounds as for every other provider type

### Requirement: Compatible shared bounded transport
The compatible model HTTP client SHALL be the shared model HTTP client used for every provider type, so compatible backends inherit redirect rejection, the fixed 10-second response-header timeout and the fixed 16 MiB per-response decompressed byte bound. No provider-specific transport setting SHALL be added.

#### Scenario: Compatible shared bounded transport
- **WHEN** a compatible backend redirects, exceeds the response byte bound or stalls response headers
- **THEN** the shared client SHALL enforce the same bounds without provider-specific settings

## MODIFIED Requirements

### Requirement: OpenAI Responses construction ignores SDK environment defaults
Each public model backed by an `openai` provider SHALL be constructed once through `providers/openai.NewResponsesWithClient` with an OpenAI Responses service assembled only from explicit options: the resolved API key, the configured or default base URL, the Gateway's shared hardened model HTTP client, and zero SDK retries. Canonical IDs and aliases SHALL resolve to the same constructed model.

#### Scenario: SDK environment is poisoned
- **WHEN** `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_ORG_ID`, `OPENAI_PROJECT_ID` and `OPENAI_CUSTOM_HEADERS` hold conflicting values and the provider has no `baseURL`
- **THEN** requests SHALL go to `https://api.openai.com/v1/responses` through the Gateway's model HTTP client with `Authorization: Bearer <resolved key>`
- **AND** requests SHALL NOT carry `OpenAI-Organization`, `OpenAI-Project` or the custom headers

#### Scenario: Unary and streaming calls reach the Responses backend
- **WHEN** an authenticated client invokes an OpenAI public model, unary or streaming
- **THEN** the backend SHALL receive `POST <baseURL>/responses` with the configured backend model ID
- **AND** the ProviderWire streaming finish part SHALL carry the output token total from the backend's completed response

#### Scenario: Responses backend exceeds the byte bound
- **WHEN** an OpenAI Responses backend response exceeds the 16 MiB model transport bound
- **THEN** the call SHALL fail without returning the response
- **AND** the client SHALL receive a fixed safe error

### Requirement: Compatible model construction over the bounded model transport
Each public model backed by an `openai-compatible` provider SHALL be constructed once through `providers/openai-compatible` with the resolved API key, the configured `baseURL`, the configured `primary.model` as the backend model ID, and the Gateway's hardened model HTTP client. The Gateway SHALL NOT retry a failed compatible request. Canonical IDs and aliases SHALL resolve to the same constructed model.

#### Scenario: Unary and streaming calls reach the configured backend
- **WHEN** an authenticated client invokes a compatible public model by canonical ID or alias, unary or streaming
- **THEN** the backend SHALL receive `POST <baseURL>/chat/completions` with `Authorization: Bearer <resolved key>` and the configured backend model ID

#### Scenario: Backend redirects
- **WHEN** a compatible backend responds with an HTTP redirect
- **THEN** the redirect target SHALL receive no request
- **AND** the client SHALL receive a fixed safe error

#### Scenario: Backend response exceeds the byte bound
- **WHEN** a compatible backend response exceeds the 16 MiB model transport bound
- **THEN** the call SHALL fail without returning the response
- **AND** the client SHALL receive a fixed safe error

## REMOVED Requirements

### Requirement: OpenAI Responses shared bounded transport and flag help
**Reason**: The `anthropic.response-header-timeout` and `anthropic.response-bytes` flags no longer exist. The bounds are fixed constants of the shared model client.
**Migration**: Replaced by "OpenAI Responses shared bounded transport".

### Requirement: Compatible shared transport flags and provider-independent help
**Reason**: The flags it describes no longer exist.
**Migration**: Replaced by "Compatible shared bounded transport".
