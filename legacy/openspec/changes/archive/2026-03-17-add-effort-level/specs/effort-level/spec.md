## ADDED Requirements

### Requirement: Effort level provider option
The Anthropic provider SHALL accept an `effort` field in `AnthropicOptions` with values `low`, `medium`, `high`, or `max`. When set, the provider SHALL include `output_config.effort` in the Anthropic API request body with the specified value.

#### Scenario: Effort level passed through to API request
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"effort":"high"}`
- **THEN** the built request params SHALL contain `output_config.effort` set to `"high"`

#### Scenario: Effort level omitted when not set
- **WHEN** caller does not set an `effort` field in provider options
- **THEN** the built request params SHALL NOT contain an `output_config` object (unless set by other features)

### Requirement: Effort beta header
The Anthropic provider SHALL append the `effort-2025-11-24` beta header to the request when effort is set.

#### Scenario: Beta header added when effort is set
- **WHEN** caller sets `{"effort":"low"}` in provider options
- **THEN** the request betas SHALL include `effort-2025-11-24`

#### Scenario: Beta header absent when effort is not set
- **WHEN** caller does not set effort in provider options
- **THEN** the request betas SHALL NOT include `effort-2025-11-24`

### Requirement: Effort is independent of thinking mode
The `effort` option SHALL be independent of the `thinking` configuration. Callers SHALL be able to set effort with any thinking mode (enabled, disabled, adaptive) or without any thinking configuration.

#### Scenario: Effort with adaptive thinking
- **WHEN** caller sets `{"thinking":{"type":"adaptive"}, "effort":"max"}`
- **THEN** the request SHALL contain both `thinking.type` set to `"adaptive"` and `output_config.effort` set to `"max"`

#### Scenario: Effort without thinking configuration
- **WHEN** caller sets `{"effort":"medium"}` without a thinking field
- **THEN** the request SHALL contain `output_config.effort` set to `"medium"` and no thinking configuration

#### Scenario: Effort with enabled thinking
- **WHEN** caller sets `{"thinking":{"type":"enabled","budgetTokens":5000}, "effort":"high"}`
- **THEN** the request SHALL contain both the thinking config and `output_config.effort` set to `"high"`
