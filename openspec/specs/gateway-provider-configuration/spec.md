# gateway-provider-configuration Specification

## Purpose
Define how the authenticated Gateway command configures named provider instances that back public models, keeping explicit configuration authoritative and outbound traffic bounded while separating configured/operator identity from supported native response identity.

## Requirements

### Requirement: Configured provider state cannot influence BYOK
All startup provider-instance configuration, secret resolution, custom endpoints and constructed account-bound models SHALL belong exclusively to configured-account access. BYOK provider construction SHALL use supported built-in adapters, fixed native destinations and supplied request credentials without consulting those instances or ambient SDK credentials. Credential-independent transport/lifecycle infrastructure SHALL remain reusable without shared mutable authentication state.

#### Scenario: Configured provider of the same type exists
- **WHEN** an OpenAI BYOK request arrives and an OpenAI instance has a configured custom endpoint and key
- **THEN** the request SHALL use the native OpenAI destination and its supplied key, never the configured instance

#### Scenario: Ambient credentials are present
- **WHEN** ambient Anthropic/OpenAI keys, base URLs or account headers conflict with a BYOK request
- **THEN** they SHALL NOT affect the destination, authentication or account headers

#### Scenario: Configured request includes BYOK
- **WHEN** a configured-access caller supplies providerOptions.gateway.byok
- **THEN** the host SHALL reject it explicitly before provider I/O rather than replace or ignore the configured credential source

### Requirement: OpenAI Responses provider configuration
The Gateway command model configuration SHALL accept `providers.<name>` instances with `type: openai`. Each instance SHALL require a non-empty `apiKeyEnv` and SHALL accept an optional `baseURL`. When `baseURL` is empty, OpenAI Responses models SHALL use `https://api.openai.com/v1`. A non-empty `baseURL` SHALL pass the same credential-free endpoint validation as other provider endpoints, including HTTPS in production deployment mode.

#### Scenario: OpenAI provider without a base URL
- **WHEN** a `type: openai` provider sets `apiKeyEnv` but no `baseURL`
- **THEN** configuration SHALL load
- **AND** its models SHALL send requests to `https://api.openai.com/v1/responses`

### Requirement: Strict OpenAI startup configuration failures
Configuration SHALL remain strict YAML, and every failure SHALL occur during startup before readiness.

#### Scenario: Strict OpenAI startup configuration failures
- **WHEN** an OpenAI provider YAML contains an unknown key
- **THEN** strict configuration SHALL fail during startup before readiness

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

### Requirement: OpenAI SDK environment exclusion and proxy compatibility
The Gateway SHALL NOT construct these models through `openai-go`'s `NewClient`, so ambient SDK configuration, including `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_ORG_ID`, `OPENAI_PROJECT_ID` and `OPENAI_CUSTOM_HEADERS`, SHALL NOT affect Gateway requests. Standard proxy variables such as `HTTPS_PROXY` still apply through the shared transport.

#### Scenario: OpenAI SDK environment exclusion and proxy compatibility
- **WHEN** OPENAI_API_KEY and OPENAI_BASE_URL conflict with configured values while HTTPS_PROXY is set
- **THEN** explicit Gateway configuration SHALL remain authoritative without openai-go NewClient while standard proxy handling SHALL still apply

### Requirement: OpenAI Responses configuration and operator identity boundaries
Authenticated discovery SHALL publish one canonical row per configured model with its name, description and specification provider grafana, plus authorized aliases/primary/fallbacks and provider-instance/provider/providerModelId facts through the gateway-configured-discovery projection. It SHALL NOT contain an OpenAI provider's API key, apiKeyEnv name, baseURL or unrelated configuration. Authorized provider-instance and configured model identifiers SHALL NOT be treated as credentials.

#### Scenario: Discovery lists OpenAI models
- **WHEN** an authorized authenticated client requests the plugin `config` route
- **THEN** the response SHALL list one canonical row per OpenAI model with explicit aliases, primary and ordered fallback facts
- **AND** it SHALL NOT contain provider credentials, secret references, baseURL or unrelated account/provider configuration

#### Scenario: OpenAI returns native identity
- **WHEN** an OpenAI backend reports identity different from its requested/canonical route
- **THEN** registered normal response identity SHALL preserve it while discovery retains authorized configured facts and public errors/operator metadata-only surfaces keep their exclusions

### Requirement: OpenAI runtime errors and operator surface exclusions
Public runtime error responses, access logs and metrics SHALL retain their existing credential/backend-detail exclusions; this feature SHALL NOT expand those surfaces.

#### Scenario: OpenAI runtime errors and operator surface exclusions
- **WHEN** an OpenAI runtime failure includes private backend details
- **THEN** public errors, access logs and metrics SHALL retain existing credential/backend-detail exclusions without expansion

### Requirement: OpenAI normal output identity and typed unary replacement
Those surface-specific restrictions SHALL NOT censor supported normal inference warnings/source ID/display or provider-reported response identity. Registered native id/modelId/timestamp SHALL survive in raw unary and stream output under their runtime contracts; typed unary response replacement SHALL remain explicitly documented. Native response modelId is distinct from configured discovery mappings and canonical operator identity.

#### Scenario: OpenAI normal output identity and typed unary replacement
- **WHEN** a successful OpenAI response reports native id/modelId/timestamp distinct from its configured route
- **THEN** raw unary/stream output SHALL retain registered identity and supported warnings/source display; typed unary replacement SHALL remain documented

### Requirement: OpenAI discovery evidence and tenant protection boundary
Configured secrets and another tenant's state SHALL remain protected. Configured discovery SHALL NOT introduce attempt/failure evidence.

#### Scenario: OpenAI discovery evidence and tenant protection boundary
- **WHEN** an authorized caller discovers configured OpenAI candidates after another request fails
- **THEN** discovery SHALL NOT add attempt/failure evidence or expose configured secrets or another tenant state

### Requirement: OpenAI-compatible provider configuration
Model configuration SHALL accept `providers.<name>` instances with `type: openai-compatible`. Each instance SHALL require a non-empty `apiKeyEnv` and `baseURL` and SHALL accept an optional `providerName`, which SHALL be rejected for `anthropic` providers and, for compatible providers, in the host-owned `gateway` option namespace. An unsupported `type` SHALL fail with `providers.<name>.type` and the list of supported types. Configuration SHALL remain strict YAML, so unknown keys still fail.

#### Scenario: Compatible provider omits the base URL
- **WHEN** a `type: openai-compatible` provider has no `baseURL`
- **THEN** the returned configuration error SHALL name `providers.<name>.baseURL`
- **AND** the process SHALL NOT become ready

#### Scenario: Provider name on an Anthropic provider
- **WHEN** a `type: anthropic` provider sets `providerName`
- **THEN** the returned configuration error SHALL name `providers.<name>.providerName`

#### Scenario: Provider named after the host controls
- **WHEN** a compatible provider sets `providerName` to `gateway`, ` gateway` or `gateway.chat`
- **THEN** the returned configuration error SHALL name `providers.<name>.providerName`

#### Scenario: Provider named grafana
- **WHEN** a compatible provider sets `providerName` to `grafana`
- **THEN** configuration SHALL load and callers SHALL be able to send `grafana` provider options to that backend

#### Scenario: Unsupported provider type
- **WHEN** a provider declares a `type` the command does not support
- **THEN** the returned configuration error SHALL name `providers.<name>.type` and list every supported provider type

#### Scenario: Valid compatible provider resolves
- **WHEN** a compatible provider sets `apiKeyEnv`, `baseURL` and `providerName`, and the referenced environment variable is non-empty
- **THEN** the resolved provider SHALL carry the API key value, base URL and provider name

### Requirement: Compatible endpoint validation before readiness
Every failure SHALL occur during startup before readiness, and a non-empty compatible `baseURL` SHALL pass the same credential-free http or https endpoint validation as other provider endpoints.

#### Scenario: Compatible endpoint validation before readiness
- **WHEN** a compatible provider configures a credential-bearing URL or an endpoint with another scheme
- **THEN** startup SHALL reject it before readiness under the shared endpoint validation

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

### Requirement: Compatible streams request usage
Streaming requests to `openai-compatible` backends SHALL send `stream_options.include_usage: true` so ProviderWire finish parts carry backend token counts. This SHALL be unconditional for Gateway-constructed compatible models. A backend that omits the usage chunk SHALL still finish the stream.

#### Scenario: Backend reports streaming usage
- **WHEN** a compatible backend returns a usage chunk for a streamed request
- **THEN** the ProviderWire finish part SHALL carry the reported output token total

#### Scenario: Stream request carries the usage option
- **WHEN** the Gateway sends a streaming request to a compatible backend
- **THEN** the request body SHALL contain `"stream_options":{"include_usage":true}`

### Requirement: Compatible stream cancellation
When a client cancels an established stream from a compatible public model, the Gateway SHALL cancel the in-flight backend request and SHALL keep serving.

#### Scenario: Client cancels an established stream
- **WHEN** a client cancels a compatible stream after `stream-start`
- **THEN** the backend request SHALL observe connection close
- **AND** the plugin SHALL continue serving requests and report healthy

### Requirement: Compatible configuration and operator identity boundaries
Authenticated discovery SHALL publish one canonical row per configured model with its name, description and specification provider grafana, plus authorized aliases/primary/fallbacks and provider-instance/effective-provider/providerModelId facts through the gateway-configured-discovery projection. The effective provider SHALL reflect the configured providerName or existing constructor default.

#### Scenario: Discovery lists compatible models
- **WHEN** an authorized authenticated client requests the plugin `config` route
- **THEN** the response SHALL list one canonical row per compatible model with explicit aliases, primary and ordered fallback facts
- **AND** it SHALL NOT contain credentials, secret references, baseURL, error bodies or unrelated account/provider configuration

#### Scenario: Backend failure carries a secret
- **WHEN** a compatible backend returns 502 with a secret marker in its error body
- **THEN** the client error, access logs and metrics SHALL retain their existing exclusions for that marker and private provider configuration
- **AND** discovery SHALL NOT reflect the runtime error or turn configured candidates into attempted/selected facts

#### Scenario: Compatible backend returns native identity
- **WHEN** a successful compatible response supplies native response identity
- **THEN** raw unary/stream output SHALL retain its registered fields while canonical routing/operator identity and typed unary replacement stay unchanged

### Requirement: Compatible discovery credential exclusions and authorized identifiers
Configured discovery SHALL NOT contain API keys, apiKeyEnv names, baseURL, backend error bodies or unrelated configuration. Authorized provider-instance/providerName/model identifiers SHALL NOT be categorically concealed. Configured discovery SHALL NOT add native bodies/headers, runtime provider identity or attempt/failure evidence.

#### Scenario: Compatible discovery credential exclusions and authorized identifiers
- **WHEN** provider configuration contains a key-looking providerName beside a secret apiKeyEnv and backend URL
- **THEN** it SHALL retain authorized provider-instance/providerName/model identifiers and exclude credentials, references, URLs, native transport and runtime evidence

### Requirement: Compatible operator exclusions do not censor normal output
Public runtime error responses, access logs and metrics SHALL retain their existing credential/backend-detail exclusions; discovery SHALL NOT enable additional operator capture or runtime diagnostic transport. Supported normal inference warnings/source ID/display and provider-reported response identity SHALL remain caller-visible under their runtime contracts despite those surface-specific configuration/operator restrictions.

#### Scenario: Compatible operator exclusions do not censor normal output
- **WHEN** a compatible request succeeds with native source display after a previous secret-bearing backend failure
- **THEN** operator surfaces SHALL keep their existing exclusions without extra capture or diagnostics, while normal supported warnings/source/identity remain caller-visible

### Requirement: Compatible native model identity and tenant protection
Returned native modelId SHALL NOT be replaced with canonical route identity. Secrets and another tenant's state SHALL remain protected independently of ordinary scalar/display strings.

#### Scenario: Compatible native model identity and tenant protection
- **WHEN** a successful compatible response supplies a native modelId different from the route
- **THEN** output SHALL retain native modelId rather than canonical replacement while secrets and another tenant state remain protected

### Requirement: OpenAI Responses shared bounded transport
OpenAI Responses models SHALL use the shared model HTTP client and inherit its redirect rejection, fixed 10-second response-header timeout and fixed 16 MiB decompressed byte bound per non-stream response under gateway-resource-policy, and leave streams to the execution time bounds. No provider-specific transport setting SHALL exist.

#### Scenario: OpenAI Responses shared bounded transport
- **WHEN** an OpenAI backend redirects, stalls its response headers, exceeds the byte bound in a unary response, or streams more than 16 MiB
- **THEN** the shared client SHALL enforce the same bounds as for every other provider type, and only the long stream SHALL succeed

### Requirement: Compatible shared bounded transport
The compatible model HTTP client SHALL be the shared model HTTP client used for every provider type, so compatible backends inherit redirect rejection, the fixed 10-second response-header timeout and the fixed 16 MiB decompressed byte bound per non-stream response, and leave streams to the execution time bounds. No provider-specific transport setting SHALL be added.

#### Scenario: Compatible shared bounded transport
- **WHEN** a compatible backend redirects, exceeds the byte bound in a unary response, stalls response headers, or streams more than 16 MiB
- **THEN** the shared client SHALL enforce the same bounds without provider-specific settings, and only the long stream SHALL succeed
