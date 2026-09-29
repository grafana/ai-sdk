## Purpose

Define how the authenticated Gateway command configures named provider instances that back public models, keeping explicit configuration authoritative, outbound model traffic bounded, and backend identity private.

## ADDED Requirements

### Requirement: OpenAI Responses provider configuration
The Gateway command model configuration SHALL accept `providers.<name>` instances with `type: openai`. Each instance SHALL require a non-empty `apiKeyEnv` and SHALL accept an optional `baseURL`. When `baseURL` is empty, OpenAI Responses models SHALL use `https://api.openai.com/v1`. A non-empty `baseURL` SHALL pass the same credential-free endpoint validation as other provider endpoints, including HTTPS in production deployment mode. Configuration SHALL remain strict YAML, and every failure SHALL occur during startup before readiness.

#### Scenario: OpenAI provider without a base URL
- **WHEN** a `type: openai` provider sets `apiKeyEnv` but no `baseURL`
- **THEN** configuration SHALL load
- **AND** its models SHALL send requests to `https://api.openai.com/v1/responses`

#### Scenario: Unsupported provider type
- **WHEN** a provider declares a `type` the command does not support
- **THEN** configuration loading SHALL fail before readiness

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

### Requirement: OpenAI Responses backend identity stays private
Discovery responses, public error responses, access logs and metrics SHALL NOT contain an OpenAI provider's instance key, `baseURL`, API key, `apiKeyEnv` name or backend model ID. Discovery SHALL keep publishing provider `grafana` with only public model IDs, names, descriptions and aliases.

#### Scenario: Discovery lists OpenAI models
- **WHEN** an authenticated client requests `/api/v1/aisdk/config`
- **THEN** the response SHALL list the OpenAI canonical and alias IDs
- **AND** SHALL NOT contain any private OpenAI provider configuration
