# gateway-provider-configuration Specification

## Purpose
Define how the authenticated Gateway command configures named provider instances that back public models, keeping explicit configuration authoritative and outbound traffic bounded while separating configured/operator identity from supported native response identity.

## Requirements

### Requirement: OpenAI Responses provider configuration
The Gateway command model configuration SHALL accept `providers.<name>` instances with `type: openai`. Each instance SHALL require a non-empty `apiKeyEnv` and SHALL accept an optional `baseURL`. When `baseURL` is empty, OpenAI Responses models SHALL use `https://api.openai.com/v1`. A non-empty `baseURL` SHALL pass the same credential-free endpoint validation as other provider endpoints, including HTTPS in production deployment mode. Configuration SHALL remain strict YAML, and every failure SHALL occur during startup before readiness.

#### Scenario: OpenAI provider without a base URL
- **WHEN** a `type: openai` provider sets `apiKeyEnv` but no `baseURL`
- **THEN** configuration SHALL load
- **AND** its models SHALL send requests to `https://api.openai.com/v1/responses`

### Requirement: OpenAI Responses construction ignores SDK environment defaults
Each public model backed by an `openai` provider SHALL be constructed once through `providers/openai.NewResponsesWithClient` with an OpenAI Responses service assembled only from explicit options: the resolved API key, the configured or default base URL, the Gateway's shared hardened model HTTP client, and zero SDK retries. The Gateway SHALL NOT construct these models through `openai-go`'s `NewClient`, so ambient SDK configuration, including `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_ORG_ID`, `OPENAI_PROJECT_ID` and `OPENAI_CUSTOM_HEADERS`, SHALL NOT affect Gateway requests. Standard proxy variables such as `HTTPS_PROXY` still apply through the shared transport. OpenAI Responses models SHALL inherit the shared redirect rejection, the `anthropic.response-header-timeout` response-header timeout and the `anthropic.response-bytes` cumulative response byte bound, and the help text for both flags SHALL state that they govern all provider types. Canonical IDs and aliases SHALL resolve to the same constructed model.

#### Scenario: SDK environment is poisoned
- **WHEN** `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_ORG_ID`, `OPENAI_PROJECT_ID` and `OPENAI_CUSTOM_HEADERS` hold conflicting values and the provider has no `baseURL`
- **THEN** requests SHALL go to `https://api.openai.com/v1/responses` through the Gateway's model HTTP client with `Authorization: Bearer <resolved key>`
- **AND** requests SHALL NOT carry `OpenAI-Organization`, `OpenAI-Project` or the custom headers

#### Scenario: Unary and streaming calls reach the Responses backend
- **WHEN** an authenticated client invokes an OpenAI public model, unary or streaming
- **THEN** the backend SHALL receive `POST <baseURL>/responses` with the configured backend model ID
- **AND** the ProviderWire streaming finish part SHALL carry the output token total from the backend's completed response

#### Scenario: Responses backend exceeds the byte bound
- **WHEN** an OpenAI Responses backend response exceeds `anthropic.response-bytes`
- **THEN** the call SHALL fail without returning the response
- **AND** the client SHALL receive a fixed safe error

### Requirement: OpenAI Responses configuration and operator identity boundaries

Discovery, public error responses, access logs and metadata-only operator metrics SHALL retain their existing exclusions for OpenAI instance keys, baseURL, API key, apiKeyEnv names and configured backend mappings/model IDs. Discovery SHALL continue publishing provider grafana and only its existing public model IDs/names/descriptions/aliases; this change SHALL NOT add configured visibility fields.

Those surface-specific restrictions SHALL NOT censor supported normal inference warnings/source ID/display or provider-reported response identity. Registered native id/modelId/timestamp SHALL survive in raw unary and stream output under their runtime contracts; typed unary response replacement SHALL remain explicitly documented. Native response modelId is distinct from configured discovery mappings and canonical operator identity. Configured secrets and another tenant's state SHALL remain protected. Attempt/failure evidence and configured discovery remain separately owned and SHALL NOT be introduced here.

#### Scenario: Discovery lists OpenAI models
- **WHEN** an authenticated caller requests config
- **THEN** discovery SHALL retain the canonical/alias public rows without private configuration or new backend mappings

#### Scenario: OpenAI returns native identity
- **WHEN** an OpenAI backend reports identity different from its requested/canonical route
- **THEN** registered normal response identity SHALL preserve it while discovery, public errors and operator metadata-only surfaces retain their existing exclusions

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
Each public model backed by an `openai-compatible` provider SHALL be constructed once through `providers/openai-compatible` with the resolved API key, the configured `baseURL`, the configured `primary.model` as the backend model ID, and the Gateway's hardened model HTTP client. That client SHALL be the same client used for Anthropic models, so compatible backends inherit redirect rejection, the `anthropic.response-header-timeout` response-header timeout and the `anthropic.response-bytes` cumulative response byte bound; no provider-specific flags SHALL be added, and the help text for both flags SHALL state that they govern all provider types. The Gateway SHALL NOT retry a failed compatible request. Canonical IDs and aliases SHALL resolve to the same constructed model.

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

### Requirement: Compatible configuration and operator identity boundaries

Discovery, public error responses, access logs and metadata-only operator metrics SHALL retain their existing exclusions for compatible instance keys, providerName, baseURL, API key, apiKeyEnv names, configured backend model IDs/mappings and backend error bodies. Discovery SHALL continue publishing provider grafana and only its existing public model IDs/names/descriptions/aliases.

Supported normal inference warnings/source ID/display and provider-reported response identity SHALL remain caller-visible under their runtime contracts despite those surface-specific configuration/operator restrictions. Returned native modelId SHALL NOT be replaced with canonical route identity. Native bodies/headers, provider identity, attempt/failure evidence and configured discovery are not added by this change. Secrets and another tenant's state SHALL remain protected independently of ordinary scalar/display strings.

#### Scenario: Discovery lists compatible models
- **WHEN** an authenticated caller requests config
- **THEN** discovery SHALL retain canonical/alias rows without private configuration or new configured mappings

#### Scenario: Backend failure carries a secret
- **WHEN** a compatible backend returns a 502 with a secret marker in its error body
- **THEN** existing client error, access logs and metrics SHALL exclude the marker and private configuration

#### Scenario: Compatible backend returns native identity
- **WHEN** a successful compatible response supplies native response identity
- **THEN** raw unary/stream output SHALL retain its registered fields while canonical routing/operator identity and typed unary replacement stay unchanged
