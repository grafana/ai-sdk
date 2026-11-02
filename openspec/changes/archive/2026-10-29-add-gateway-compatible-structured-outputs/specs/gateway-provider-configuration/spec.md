## ADDED Requirements

### Requirement: Compatible structured-output capability setting
Model configuration SHALL accept an optional boolean `supportsStructuredOutputs` on `providers.<name>` instances. It SHALL default to `false`. Setting it to `true` on an `anthropic` or `openai` provider SHALL fail during startup, before readiness, with an error that names `providers.<name>.supportsStructuredOutputs`. Configuration SHALL remain strict YAML. Configured discovery SHALL NOT expose the capability.

#### Scenario: Compatible provider enables structured outputs
- **WHEN** a `type: openai-compatible` provider sets `supportsStructuredOutputs: true` and its other settings are valid
- **THEN** configuration SHALL load
- **AND** the resolved provider SHALL carry the enabled capability with its API key value, base URL and provider name

#### Scenario: Compatible provider omits the setting
- **WHEN** a `type: openai-compatible` provider does not set `supportsStructuredOutputs`
- **THEN** configuration SHALL load and the resolved provider SHALL carry the capability disabled

#### Scenario: Structured outputs enabled on a non-compatible provider
- **WHEN** a `type: anthropic` or `type: openai` provider sets `supportsStructuredOutputs: true`
- **THEN** the returned configuration error SHALL name `providers.<name>.supportsStructuredOutputs` and state that the setting is only supported for openai-compatible providers
- **AND** the process SHALL NOT become ready

#### Scenario: Explicit false on a non-compatible provider
- **WHEN** a `type: anthropic` or `type: openai` provider sets `supportsStructuredOutputs: false`
- **THEN** configuration SHALL load as if the setting were absent

## MODIFIED Requirements

### Requirement: Compatible model construction over the bounded model transport
Each public model backed by an `openai-compatible` provider SHALL be constructed once through `providers/openai-compatible` with the resolved API key, the configured `baseURL`, the configured `primary.model` as the backend model ID, the configured `supportsStructuredOutputs` capability, and the Gateway's hardened model HTTP client. The Gateway SHALL NOT retry a failed compatible request. Canonical IDs and aliases SHALL resolve to the same constructed model.

#### Scenario: Unary and streaming calls reach the configured backend
- **WHEN** an authenticated client invokes a compatible public model by canonical ID or alias, unary or streaming
- **THEN** the backend SHALL receive `POST <baseURL>/chat/completions` with `Authorization: Bearer <resolved key>` and the configured backend model ID

#### Scenario: Structured-output capability reaches each compatible candidate
- **WHEN** a compatible provider used as a primary or configured fallback candidate sets `supportsStructuredOutputs: true`
- **THEN** the model constructed for that candidate SHALL send JSON-schema response formats
- **AND** a compatible provider without the setting SHALL be constructed with the capability disabled
- **AND** when a primary and a fallback candidate have opposite settings, each candidate's backend request SHALL use the response format of its own provider's setting

#### Scenario: Backend redirects
- **WHEN** a compatible backend responds with an HTTP redirect
- **THEN** the redirect target SHALL receive no request
- **AND** the client SHALL receive a fixed safe error

#### Scenario: Backend response exceeds the byte bound
- **WHEN** a compatible backend response exceeds the 16 MiB model transport bound
- **THEN** the call SHALL fail without returning the response
- **AND** the client SHALL receive a fixed safe error
