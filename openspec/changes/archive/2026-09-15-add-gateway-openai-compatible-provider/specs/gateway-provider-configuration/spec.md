## Purpose

Define how the authenticated Gateway command configures named provider instances that back public models, keeping explicit configuration authoritative, outbound model traffic bounded, and backend identity private.

## ADDED Requirements

### Requirement: OpenAI-compatible provider configuration
The Gateway command model configuration SHALL accept `providers.<name>` instances with `type: openai-compatible`. Each instance SHALL require a non-empty `apiKeyEnv` and a non-empty `baseURL`, and SHALL accept an optional `providerName`. `providerName` SHALL be rejected for `anthropic` providers. An unsupported `type` SHALL fail with `providers.<name>.type` and the list of supported types. Configuration SHALL remain strict YAML, so unknown keys still fail. Every failure SHALL occur during startup before readiness, and a non-empty compatible `baseURL` SHALL pass the same credential-free endpoint validation as other provider endpoints, including HTTPS in production deployment mode.

#### Scenario: Compatible provider omits the base URL
- **WHEN** a `type: openai-compatible` provider has no `baseURL`
- **THEN** the returned configuration error SHALL name `providers.<name>.baseURL`
- **AND** the process SHALL NOT become ready

#### Scenario: Provider name on an Anthropic provider
- **WHEN** a `type: anthropic` provider sets `providerName`
- **THEN** the returned configuration error SHALL name `providers.<name>.providerName`

#### Scenario: Unsupported provider type
- **WHEN** a provider declares a `type` the command does not support
- **THEN** the returned configuration error SHALL name `providers.<name>.type` and list every supported provider type

#### Scenario: Valid compatible provider resolves
- **WHEN** a compatible provider sets `apiKeyEnv`, `baseURL` and `providerName`, and the referenced environment variable is non-empty
- **THEN** the resolved provider SHALL carry the API key value, base URL and provider name

### Requirement: Compatible model construction over the bounded model transport
Each public model backed by an `openai-compatible` provider SHALL be constructed once through `providers/openai-compatible` with the resolved API key, the configured `baseURL`, the configured `primary.model` as the backend model ID, and the Gateway's hardened model HTTP client. That client SHALL be the same client used for Anthropic models, so compatible backends inherit redirect rejection, the `anthropic.response-header-timeout` response-header timeout and the `anthropic.response-bytes` cumulative response byte bound; no provider-specific flags SHALL be added, and the help text for both flags SHALL state that they govern Anthropic and OpenAI-compatible providers. The Gateway SHALL NOT retry a failed compatible request. Canonical IDs and aliases SHALL resolve to the same constructed model.

#### Scenario: Unary and streaming calls reach the configured backend
- **WHEN** an authenticated client invokes a compatible public model by canonical ID or alias, unary or streaming
- **THEN** the backend SHALL receive `POST <baseURL>/chat/completions` with `Authorization: Bearer <resolved key>` and the configured backend model ID

#### Scenario: Backend redirects
- **WHEN** a compatible backend responds with an HTTP redirect
- **THEN** the redirect target SHALL receive no request
- **AND** the client SHALL receive a fixed safe error

#### Scenario: Backend response exceeds the byte bound
- **WHEN** a compatible backend response exceeds `anthropic.response-bytes`
- **THEN** the call SHALL fail without returning the response
- **AND** the client SHALL receive a fixed safe error

### Requirement: Compatible streams request usage
Streaming requests to `openai-compatible` backends SHALL send `stream_options.include_usage: true` so ProviderWire finish parts carry backend token counts. This SHALL be unconditional for Gateway-constructed compatible models. A backend that omits the usage chunk SHALL still finish the stream.

#### Scenario: Backend reports streaming usage
- **WHEN** a compatible backend returns a usage chunk for a streamed request
- **THEN** the ProviderWire finish part SHALL carry the reported output token total

#### Scenario: Stream request carries the usage option
- **WHEN** the Gateway sends a streaming request to a compatible backend
- **THEN** the request body SHALL contain `"stream_options":{"include_usage":true}`

### Requirement: Compatible stream cancellation
When a client cancels an established stream from a compatible public model, the Gateway SHALL cancel the in-flight backend request and SHALL remain ready.

#### Scenario: Client cancels an established stream
- **WHEN** a client cancels a compatible stream after `stream-start`
- **THEN** the backend request SHALL observe connection close
- **AND** `/ready` SHALL continue to succeed

### Requirement: Compatible backend identity stays private
Discovery responses, public error responses, access logs and metrics SHALL NOT contain a compatible provider's instance key, `providerName`, `baseURL`, API key, `apiKeyEnv` name, backend model ID, or backend error body. Discovery SHALL keep publishing provider `grafana` with only public model IDs, names, descriptions and aliases.

#### Scenario: Discovery lists compatible models
- **WHEN** an authenticated client requests `/api/v1/aisdk/config`
- **THEN** the response SHALL list the compatible canonical and alias IDs
- **AND** SHALL NOT contain any private compatible provider configuration

#### Scenario: Backend failure carries a secret
- **WHEN** a compatible backend returns `502` with a secret marker in its error body
- **THEN** the client error, access logs and metrics SHALL NOT contain the marker or any private provider configuration
