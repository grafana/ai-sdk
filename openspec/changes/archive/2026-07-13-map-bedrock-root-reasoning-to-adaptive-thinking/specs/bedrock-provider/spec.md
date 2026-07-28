## ADDED Requirements

### Requirement: Root reasoning resolution for Anthropic models

When `provider.CallOptions.Reasoning` is a custom level other than `none` and the Bedrock model ID identifies an Anthropic model, the provider SHALL select thinking behavior from the registered upstream model capability set. Models whose IDs contain `claude-opus-4-6`, `claude-opus-4-7`, `claude-opus-4-8`, `claude-sonnet-4-6`, `claude-fable-5`, or `claude-sonnet-5` SHALL use adaptive thinking; older and unknown Anthropic models SHALL use budget-token thinking.

For adaptive models, reasoning levels SHALL map to `additionalModelRequestFields.output_config.effort` as follows: `minimal` to `low`, `low` to `low`, `medium` to `medium`, `high` to `high`, and `xhigh` to `max`. A mapping that changes the level name SHALL emit a compatibility warning. For budget-based models, the provider SHALL derive a token budget from the model's maximum output tokens and increase `inferenceConfig.maxTokens` by that budget.

For custom reasoning other than `none`, non-zero fields from an explicit provider `reasoningConfig` SHALL override the corresponding derived fields while unspecified fields remain derived. If the merged type is `disabled`, derived budget and effort SHALL be removed. Anthropic root reasoning `none` SHALL replace an explicit partial reasoning config with disabled thinking.

#### Scenario: Adaptive-capable model receives adaptive thinking and effort

- **WHEN** root reasoning is `high` for `anthropic.claude-sonnet-4-6-v1:0`
- **THEN** `additionalModelRequestFields.thinking` SHALL equal `{type: "adaptive"}`
- **AND** `additionalModelRequestFields.output_config.effort` SHALL equal `high`
- **AND** the request SHALL NOT include a reasoning budget or budget-derived `inferenceConfig.maxTokens`

#### Scenario: Older model retains budget-token thinking

- **WHEN** root reasoning is `high` for `anthropic.claude-sonnet-4-5-20250929-v1:0`
- **THEN** `additionalModelRequestFields.thinking.type` SHALL equal `enabled`
- **AND** `additionalModelRequestFields.thinking.budget_tokens` SHALL equal `38400`
- **AND** `inferenceConfig.maxTokens` SHALL equal `42496`
- **AND** `additionalModelRequestFields.output_config.effort` SHALL be omitted

#### Scenario: Older Sonnet models use their capability maximum

- **WHEN** root reasoning is `high` for another Claude Sonnet 4.x model
- **THEN** the reasoning budget SHALL equal `38400`, derived from a `64000` maximum

#### Scenario: Older Opus models use their capability maximum

- **WHEN** root reasoning is `high` for Claude Opus 4.1 or another Claude Opus 4.x model
- **THEN** the reasoning budget SHALL equal `19200`, derived from a `32000` maximum

#### Scenario: Adaptive effort compatibility mapping

- **WHEN** root reasoning is `minimal` for an adaptive-capable Anthropic Bedrock model
- **THEN** `additionalModelRequestFields.output_config.effort` SHALL equal `low`
- **AND** the provider SHALL emit a compatibility warning for `reasoning`

#### Scenario: Provider-default reasoning is omitted

- **WHEN** root reasoning is unset or `provider-default`
- **THEN** the provider SHALL NOT derive thinking or effort configuration from root reasoning

#### Scenario: Reasoning none disables Anthropic thinking

- **WHEN** root reasoning is `none` for an Anthropic Bedrock model
- **THEN** the derived reasoning configuration SHALL disable thinking
- **AND** the request SHALL NOT include a derived reasoning budget or effort

#### Scenario: Partial provider config preserves adaptive derivation

- **WHEN** root reasoning is `high` for an adaptive-capable Anthropic model and provider `reasoningConfig.display` is `summarized`
- **THEN** the request SHALL use adaptive thinking with display `summarized`
- **AND** `additionalModelRequestFields.output_config.effort` SHALL equal `high`

#### Scenario: Explicit enabled config retains derived effort

- **WHEN** root reasoning is `high` for an adaptive-capable Anthropic model and provider reasoning config sets type `enabled` with a token budget
- **THEN** the request SHALL use the explicit enabled type and token budget
- **AND** `additionalModelRequestFields.output_config.effort` SHALL equal `high`

#### Scenario: Disabled provider config clears derived values

- **WHEN** custom root reasoning is combined with provider reasoning config type `disabled`
- **THEN** the request SHALL omit derived reasoning budget and effort

#### Scenario: Reasoning none overrides partial provider config

- **WHEN** root reasoning is `none` for an Anthropic model and provider reasoning config only sets display
- **THEN** the request SHALL omit thinking and effort fields
