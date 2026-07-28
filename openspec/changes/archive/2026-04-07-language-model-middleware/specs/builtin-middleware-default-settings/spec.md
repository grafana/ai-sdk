## ADDED Requirements

### Requirement: Apply default call options
`DefaultSettings` SHALL accept a settings struct and return a `Middleware` whose `TransformParams` hook merges the settings as defaults into `provider.CallOptions`.

Caller-provided values SHALL take precedence over defaults. Only fields not set by the caller (nil pointers, zero-length slices, nil maps) SHALL be filled with defaults.

Supported default fields: `MaxOutputTokens`, `Temperature`, `TopP`, `TopK`, `PresencePenalty`, `FrequencyPenalty`, `StopSequences`, `ResponseFormat`, `Seed`, `Headers`, `ProviderOptions`, `Tools`, `ToolChoice`.

#### Scenario: Default temperature applied
- **WHEN** `DefaultSettings` is configured with `Temperature: ptr(0.7)`
- **AND** the caller does not set `Temperature` in `CallOptions`
- **THEN** `CallOptions.Temperature` SHALL be `0.7` when it reaches the model

#### Scenario: Caller value takes precedence
- **WHEN** `DefaultSettings` is configured with `Temperature: ptr(0.7)`
- **AND** the caller sets `Temperature: ptr(0.3)` in `CallOptions`
- **THEN** `CallOptions.Temperature` SHALL be `0.3` when it reaches the model

#### Scenario: Multiple default fields
- **WHEN** `DefaultSettings` is configured with `Temperature`, `MaxOutputTokens`, and `StopSequences`
- **AND** the caller only sets `Temperature`
- **THEN** `MaxOutputTokens` and `StopSequences` SHALL use the defaults
- **AND** `Temperature` SHALL use the caller's value

#### Scenario: ProviderOptions merge
- **WHEN** `DefaultSettings` is configured with `ProviderOptions` containing key `"anthropic"`
- **AND** the caller also provides `ProviderOptions` with key `"anthropic"`
- **THEN** the caller's `"anthropic"` value SHALL take precedence (no deep merge)
- **WHEN** the caller provides `ProviderOptions` with key `"openai"` (not in defaults)
- **THEN** both `"anthropic"` (from caller) and `"openai"` (from caller) SHALL be present

#### Scenario: Headers merge
- **WHEN** `DefaultSettings` is configured with default `Headers`
- **AND** the caller provides additional `Headers`
- **THEN** caller headers SHALL override default headers for matching keys
- **AND** non-overlapping default headers SHALL be preserved
