## ADDED Requirements

### Requirement: Recording uses response model identity when available

Sigil recording SHALL use backend response metadata as the canonical generation model identity when a successful provider response supplies both provider and model ID. When response metadata omits either provider or model ID, recording SHALL preserve the model identity from `GenerationStart`.

#### Scenario: Generate response overrides transport model identity

- **GIVEN** a wrapped model whose `Provider()` returns `grafana` and `ModelID()` returns `claude-sonnet-4-5-20250929`
- **WHEN** `RecordingMiddleware` records a successful `DoGenerate` result whose `Response.Provider` is `anthropic` and `Response.ModelID` is `claude-sonnet-4-5-20250929`
- **THEN** the resulting Sigil generation's `Model.Provider` SHALL equal `anthropic`
- **AND** the resulting Sigil generation's `Model.Name` SHALL equal `claude-sonnet-4-5-20250929`

#### Scenario: Generate response without provider keeps seed identity

- **GIVEN** a wrapped model whose `Provider()` returns `grafana` and `ModelID()` returns `claude-sonnet-4-5-20250929`
- **WHEN** `RecordingMiddleware` records a successful `DoGenerate` result whose `Response.ModelID` is populated but `Response.Provider` is empty
- **THEN** the resulting Sigil generation's `Model.Provider` SHALL equal `grafana`
- **AND** the resulting Sigil generation's `Model.Name` SHALL equal `claude-sonnet-4-5-20250929`

#### Scenario: Stream response metadata overrides transport model identity

- **GIVEN** a stream recorder seeded with `GenerationStart.Model.Provider` equal to `grafana` and `GenerationStart.Model.Name` equal to `claude-sonnet-4-5-20250929`
- **WHEN** `StreamRecorder.Observe` receives `provider.StreamPart{Type: PartResponseMeta, Provider: "anthropic", ModelID: "claude-sonnet-4-5-20250929"}` before stream completion
- **THEN** `StreamRecorder.Generation().Model.Provider` SHALL equal `anthropic`
- **AND** `StreamRecorder.Generation().Model.Name` SHALL equal `claude-sonnet-4-5-20250929`

#### Scenario: Stream response metadata without provider keeps seed identity

- **GIVEN** a stream recorder seeded with `GenerationStart.Model.Provider` equal to `grafana` and `GenerationStart.Model.Name` equal to `claude-sonnet-4-5-20250929`
- **WHEN** `StreamRecorder.Observe` receives `provider.StreamPart{Type: PartResponseMeta, ModelID: "claude-sonnet-4-5-20250929"}` without a provider
- **THEN** `StreamRecorder.Generation().Model.Provider` SHALL equal `grafana`
- **AND** `StreamRecorder.Generation().Model.Name` SHALL equal `claude-sonnet-4-5-20250929`

### Requirement: Recording preserves transport identity metadata

When response metadata changes the canonical generation model identity from the wrapped model identity, Sigil recording SHALL add generic transport identity metadata. The metadata SHALL include `ai_sdk.transport.provider` with the wrapped model provider and `ai_sdk.transport.model` with the wrapped model ID. Recording SHALL NOT add this metadata when the final canonical model identity matches the wrapped model identity or when response metadata is incomplete.

#### Scenario: Generate records transport metadata when response identity differs

- **GIVEN** a wrapped model whose `Provider()` returns `grafana` and `ModelID()` returns `claude-sonnet-4-5-20250929`
- **WHEN** `RecordingMiddleware` records a successful `DoGenerate` result whose `Response.Provider` is `anthropic` and `Response.ModelID` is `claude-sonnet-4-5-20250929`
- **THEN** the resulting Sigil generation metadata SHALL contain `ai_sdk.transport.provider` equal to `grafana`
- **AND** the resulting Sigil generation metadata SHALL contain `ai_sdk.transport.model` equal to `claude-sonnet-4-5-20250929`

#### Scenario: Stream records transport metadata when response identity differs

- **GIVEN** a stream recorder seeded with `GenerationStart.Model.Provider` equal to `grafana` and `GenerationStart.Model.Name` equal to `claude-sonnet-4-5-20250929`
- **WHEN** `StreamRecorder.Observe` receives `provider.StreamPart{Type: PartResponseMeta, Provider: "anthropic", ModelID: "claude-sonnet-4-5-20250929"}` before stream completion
- **THEN** `StreamRecorder.Generation().Metadata` SHALL contain `ai_sdk.transport.provider` equal to `grafana`
- **AND** `StreamRecorder.Generation().Metadata` SHALL contain `ai_sdk.transport.model` equal to `claude-sonnet-4-5-20250929`

#### Scenario: Direct provider does not record transport metadata

- **GIVEN** a wrapped model whose `Provider()` returns `anthropic` and `ModelID()` returns `claude-sonnet-4-5-20250929`
- **WHEN** `RecordingMiddleware` records a successful response whose response provider and model match the wrapped model identity
- **THEN** the resulting Sigil generation metadata SHALL NOT contain `ai_sdk.transport.provider`
- **AND** the resulting Sigil generation metadata SHALL NOT contain `ai_sdk.transport.model`
