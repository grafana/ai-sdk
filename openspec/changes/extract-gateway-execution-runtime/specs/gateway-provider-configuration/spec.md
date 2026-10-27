# Spec Delta

## MODIFIED Requirements

### Requirement: OpenAI-compatible provider configuration
The Gateway command model configuration SHALL accept `providers.<name>` instances with `type: openai-compatible`. Each instance SHALL require a non-empty `apiKeyEnv` and a non-empty `baseURL`, and SHALL accept an optional `providerName`. `providerName` SHALL be rejected for `anthropic` providers. A compatible `providerName` whose option namespace, the trimmed text before its first `.`, is `gateway` SHALL be rejected, because the host owns call-level `gateway` controls. An unsupported `type` SHALL fail with `providers.<name>.type` and the list of supported types. Configuration SHALL remain strict YAML, so unknown keys still fail.

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
