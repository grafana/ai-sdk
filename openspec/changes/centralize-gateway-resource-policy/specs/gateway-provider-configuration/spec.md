# Spec Delta

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
- **WHEN** an OpenAI Responses backend response exceeds the shared provider response byte bound
- **THEN** the call SHALL fail without returning the response
- **AND** the client SHALL receive a fixed safe error

### Requirement: OpenAI Responses shared bounded transport and flag help
OpenAI Responses models SHALL inherit the shared redirect rejection, response-header timeout and cumulative response byte bound under gateway-resource-policy. These bounds SHALL be fixed for all provider types; no provider-specific setting SHALL exist.

#### Scenario: OpenAI Responses shared bounded transport and flag help
- **WHEN** an OpenAI backend redirects or exceeds the configured response timeout/byte bound
- **THEN** the shared hardened client SHALL enforce the same all-provider bounds without a provider-specific setting

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
- **WHEN** a compatible backend response exceeds the shared provider response byte bound
- **THEN** the call SHALL fail without returning the response
- **AND** the client SHALL receive a fixed safe error

### Requirement: Compatible shared transport flags and provider-independent help
The compatible model HTTP client SHALL be the same client used for Anthropic models, so compatible backends inherit redirect rejection, the response-header timeout and the cumulative response byte bound under gateway-resource-policy; no provider-specific setting SHALL be added.

#### Scenario: Compatible shared transport flags and provider-independent help
- **WHEN** a compatible backend redirects, exceeds the response-byte bound or stalls response headers
- **THEN** the same shared client SHALL enforce the limits without provider-specific settings
