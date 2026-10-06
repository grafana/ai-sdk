## MODIFIED Requirements

### Requirement: Reasoning none disables thinking
When `CallOptions.Reasoning` is `"none"`, the Anthropic provider SHALL set `thinking: disabled`, except on models that support `between_tools` thinking. Those models reject disabled thinking, so the provider SHALL set `thinking: between_tools`, their lowest setting, without a warning, as `@ai-sdk/anthropic` 4.0.67 does. No `output_config.effort` SHALL be set in either case.

#### Scenario: Reasoning none on any model
- **WHEN** `CallOptions.Reasoning` is `"none"` and the model does not support `between_tools` thinking
- **THEN** the request SHALL contain `thinking.type` set to `"disabled"`
- **AND** no `output_config.effort` SHALL be present
- **AND** no effort beta header SHALL be added

#### Scenario: Reasoning none on Sonnet 5.5
- **WHEN** `CallOptions.Reasoning` is `"none"` and the model is `claude-sonnet-5-5`
- **THEN** the request SHALL contain `thinking.type` set to `"between_tools"`
- **AND** no `output_config.effort` SHALL be present
- **AND** no warning SHALL be emitted

### Requirement: Adaptive thinking display
The Anthropic provider SHALL accept a `display` field in `ThinkingConfig` with values `"summarized"` or `"omitted"`. When set and the configured `type` is `"adaptive"`, the provider SHALL include `thinking.display` in the Anthropic API request body with the specified value. The `display` field SHALL be ignored when `type` is `"enabled"`, `"disabled"` or `"between_tools"`.

#### Scenario: Display set on adaptive thinking
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"thinking":{"type":"adaptive","display":"summarized"}}`
- **THEN** the built request params SHALL contain `thinking.type` set to `"adaptive"` and `thinking.display` set to `"summarized"`

#### Scenario: Display omitted on adaptive thinking
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"thinking":{"type":"adaptive"}}` (no display)
- **THEN** the built request params SHALL contain `thinking.type` set to `"adaptive"` and SHALL NOT contain `thinking.display`

#### Scenario: Display ignored on enabled thinking
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"thinking":{"type":"enabled","budgetTokens":5000,"display":"omitted"}}`
- **THEN** the built request params SHALL NOT contain `thinking.display`

#### Scenario: Display ignored on between_tools thinking
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"thinking":{"type":"between_tools","display":"summarized"}}`
- **THEN** the built request params SHALL contain `thinking.type` set to `"between_tools"` and SHALL NOT contain `thinking.display`
