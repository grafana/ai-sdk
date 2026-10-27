# Spec Delta

## MODIFIED Requirements

### Requirement: OpenAI Responses configuration and operator identity boundaries
Authenticated discovery SHALL publish one canonical row per configured model with its name, description and specification provider grafana, plus authorized aliases/primary/fallbacks and provider-instance/provider/providerModelId facts through the gateway-configured-discovery projection. It SHALL NOT contain an OpenAI provider's API key, apiKeyEnv name, baseURL or unrelated configuration. Authorized provider-instance and configured model identifiers SHALL NOT be treated as credentials.

#### Scenario: Discovery lists OpenAI models
- **WHEN** an authorized authenticated client requests the plugin `config` route
- **THEN** the response SHALL list one canonical row per OpenAI model with explicit aliases, primary and ordered fallback facts
- **AND** it SHALL NOT contain provider credentials, secret references, baseURL or unrelated account/provider configuration

#### Scenario: OpenAI returns native identity
- **WHEN** an OpenAI backend reports identity different from its requested/canonical route
- **THEN** registered normal response identity SHALL preserve it while discovery retains authorized configured facts and public errors/operator metadata-only surfaces keep their exclusions

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

### Requirement: Compatible stream cancellation
When a client cancels an established stream from a compatible public model, the Gateway SHALL cancel the in-flight backend request and SHALL keep serving.

#### Scenario: Client cancels an established stream
- **WHEN** a client cancels a compatible stream after `stream-start`
- **THEN** the backend request SHALL observe connection close
- **AND** the plugin SHALL continue serving requests and report healthy

### Requirement: Compatible endpoint validation before readiness
Every failure SHALL occur during startup before readiness, and a non-empty compatible `baseURL` SHALL pass the same credential-free http or https endpoint validation as other provider endpoints.

#### Scenario: Compatible endpoint validation before readiness
- **WHEN** a compatible provider configures a credential-bearing URL or an endpoint with another scheme
- **THEN** startup SHALL reject it before readiness under the shared endpoint validation
