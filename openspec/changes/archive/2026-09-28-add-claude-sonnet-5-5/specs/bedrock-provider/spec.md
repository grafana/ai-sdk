## MODIFIED Requirements

### Requirement: Root reasoning resolution for Anthropic models

When `provider.CallOptions.Reasoning` is a custom level other than `none` and the Bedrock model is identified as Anthropic (including by explicit constructor family or a profile ARN with explicit reasoning budget), the provider SHALL select thinking behavior from the registered upstream model capability set. Models whose IDs contain `claude-opus-4-6`, `claude-opus-4-7`, `claude-opus-4-8`, `claude-sonnet-4-6`, `claude-fable-5`, or `claude-sonnet-5` SHALL use adaptive thinking. Unknown IDs containing `claude-` but not matching a known or legacy Claude family SHALL also use adaptive thinking with the pinned 128000-token capability maximum; known older and legacy Claude families SHALL use their pinned budget-token capability maxima. Unknown non-Claude IDs identified as Anthropic (for example, by explicit family on an opaque profile ARN) SHALL use conservative budget-token thinking with a 4096-token capability maximum.

For adaptive models, reasoning levels SHALL map to `additionalModelRequestFields.output_config.effort` as follows: `minimal` to `low`, `low` to `low`, `medium` to `medium`, `high` to `high`, and `xhigh` to `max`. A mapping that changes the level name SHALL emit a compatibility warning. For budget-based models, the provider SHALL derive a token budget from the model's maximum output tokens and increase `inferenceConfig.maxTokens` by that budget.

For custom reasoning other than `none`, non-zero fields from an explicit provider `reasoningConfig` SHALL override the corresponding derived fields while unspecified fields remain derived. A raw JSON `budgetTokens` field explicitly set to zero SHALL also override a derived budget and produce `thinking.budget_tokens = 0`; the zero-valued typed Go field with `omitempty` represents omission. If the merged type is `disabled`, derived budget and effort SHALL be removed. Anthropic root reasoning `none` SHALL replace an explicit partial reasoning config with disabled thinking, except on models that support `between_tools` thinking (IDs containing `claude-sonnet-5-5`), where it SHALL use `between_tools` thinking. This is an intentional deviation from `@ai-sdk/amazon-bedrock` 5.0.99, recorded in `test/conformance/upstream.yaml`: upstream sends disabled thinking, which omits `thinking` and lets the model run adaptive thinking at its default effort.

#### Scenario: Adaptive-capable model receives adaptive thinking and effort

- **WHEN** root reasoning is `high` for `anthropic.claude-sonnet-4-6-v1:0`
- **THEN** `additionalModelRequestFields.thinking` SHALL equal `{type: "adaptive"}`
- **AND** `additionalModelRequestFields.output_config.effort` SHALL equal `high`
- **AND** the request SHALL NOT include a reasoning budget or budget-derived `inferenceConfig.maxTokens`

#### Scenario: Dated and regional IDs retain capability-derived budgets

- **WHEN** root reasoning is `high` for `us.anthropic.claude-sonnet-4-5-20250929-v1:0` or `anthropic.claude-opus-4-1-20250805-v1:0`
- **THEN** budget-token thinking uses the registered capability maxima of 64000 or 32000 respectively and the corresponding 38400 or 19200 high budget; explicit nonzero budget overrides the derived budget

#### Scenario: Raw zero budget retains explicit presence

- **WHEN** an application inference-profile ARN has raw `reasoningConfig = {type: "enabled", budgetTokens: 0}`
- **THEN** the provider identifies the Anthropic family and emits `thinking = {type: "enabled", budget_tokens: 0}` with default `inferenceConfig.maxTokens = 4096`
- **AND** a raw zero budget overrides a derived root-reasoning budget on an Anthropic model

#### Scenario: Older Opus models use their capability maximum

- **WHEN** root reasoning is `high` for Claude Opus 4.1 or another older Claude Opus 4.x model with a `32000` capability maximum
- **THEN** the reasoning budget SHALL equal `19200`, derived from that maximum

#### Scenario: Older model retains budget-token thinking

- **WHEN** root reasoning is `high` for `anthropic.claude-sonnet-4-5-20250929-v1:0`
- **THEN** `additionalModelRequestFields.thinking.type` SHALL equal `enabled`
- **AND** `additionalModelRequestFields.thinking.budget_tokens` SHALL equal `38400`
- **AND** `inferenceConfig.maxTokens` SHALL equal `42496`
- **AND** `additionalModelRequestFields.output_config.effort` SHALL be omitted

#### Scenario: Unknown Claude ID uses adaptive fallback

- **WHEN** root reasoning is `high` for an unrecognized Anthropic model ID containing `claude-` but not matching a known or legacy Claude family
- **THEN** the pinned capability fallback has a 128000-token maximum and the request uses adaptive thinking with `output_config.effort = high`, not a derived token budget

#### Scenario: Unknown non-Claude Anthropic profile uses conservative fallback

- **WHEN** root reasoning is `high` for an opaque non-Claude application inference-profile ARN with explicit Anthropic constructor family and no explicit budget
- **THEN** the pinned capability fallback has a 4096-token maximum and the request uses enabled budget-token thinking with a high-level budget derived from that maximum, not adaptive thinking

#### Scenario: Known legacy Claude retains conservative budget fallback

- **WHEN** root reasoning is `high` for a legacy Claude 3 model ID that does not match a newer Claude capability family
- **THEN** the pinned capability maximum is 4096 and the request uses enabled budget-token thinking rather than the unknown-Claude adaptive fallback

#### Scenario: Older Sonnet models use their capability maximum

- **WHEN** root reasoning is `high` for a non-adaptive Claude Sonnet 4.x model such as `anthropic.claude-sonnet-4-20250514-v1:0` (not Sonnet 4.6)
- **THEN** the reasoning budget SHALL equal `38400`, derived from a `64000` maximum

#### Scenario: Opus 4 and 4.1 use their capability maximum

- **WHEN** root reasoning is `high` for Claude Opus 4 or 4.1, such as `anthropic.claude-opus-4-20250514-v1:0` or `anthropic.claude-opus-4-1-20250805-v1:0`
- **THEN** the reasoning budget SHALL equal `19200`, derived from a `32000` maximum

#### Scenario: Opus 4.5 uses its larger capability maximum

- **WHEN** root reasoning is `high` for `anthropic.claude-opus-4-5-20251101-v1:0`
- **THEN** the reasoning budget SHALL equal `38400`, derived from a `64000` maximum, without adaptive thinking

#### Scenario: Adaptive effort compatibility mapping

- **WHEN** root reasoning is `minimal` for an adaptive-capable Anthropic Bedrock model
- **THEN** `additionalModelRequestFields.output_config.effort` SHALL equal `low`
- **AND** the provider SHALL emit a compatibility warning for `reasoning`

#### Scenario: Provider-default reasoning is omitted

- **WHEN** root reasoning is unset or `provider-default`
- **THEN** the provider SHALL NOT derive thinking or effort configuration from root reasoning

#### Scenario: Reasoning none disables Anthropic thinking

- **WHEN** root reasoning is `none` for an Anthropic Bedrock model that does not support `between_tools` thinking
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

#### Scenario: Reasoning none on Sonnet 5.5 uses between_tools thinking

- **WHEN** root reasoning is `none` for `global.anthropic.claude-sonnet-5-5`, with or without a partial provider reasoning config
- **THEN** `additionalModelRequestFields.thinking` SHALL equal `{type: "between_tools"}`
- **AND** the request SHALL NOT include a derived reasoning budget or effort

## ADDED Requirements

### Requirement: Converse Claude models that reject disabled thinking and forced tool use

For Anthropic Bedrock model IDs containing `claude-sonnet-5-5`, the Converse adapter SHALL avoid request shapes the model rejects, following `@ai-sdk/amazon-bedrock` 5.0.99 unless noted. A `required` tool choice SHALL be sent as `auto`, and a named tool choice SHALL be sent as `auto` with only the named tool, each with an unsupported `toolChoice` warning. A JSON schema response SHALL use the system-prompt JSON instruction instead of the forced JSON tool, whatever the structured-output mode, unless native `outputFormat` output is selected.

As a Go extension, `reasoningConfig.type` SHALL also accept `between_tools`. It SHALL be sent as `thinking: {type: "between_tools"}` without `display` or `budget_tokens`, SHALL count as active thinking for sampling-parameter removal, and SHALL lower `maxReasoningEffort` `xhigh` or `max` to `high` with an unsupported warning for feature `providerOptions.amazonBedrock.reasoningConfig.maxReasoningEffort`. Upstream 5.0.99 has no `between_tools` type.

#### Scenario: Required tool choice on Sonnet 5.5

- **WHEN** `global.anthropic.claude-sonnet-5-5` is called with two function tools and tool choice `required`
- **THEN** `toolConfig.toolChoice` SHALL be `auto` and both tools SHALL be sent
- **AND** an unsupported `toolChoice` warning SHALL be emitted

#### Scenario: Named tool choice on Sonnet 5.5

- **WHEN** `global.anthropic.claude-sonnet-5-5` is called with tools `weather` and `search` and tool choice `tool` named `search`
- **THEN** `toolConfig.toolChoice` SHALL be `auto` and only `search` SHALL be sent

#### Scenario: JSON response without native output on Sonnet 5.5

- **WHEN** `global.anthropic.claude-sonnet-5-5` receives a JSON schema response with no caller tools and `structuredOutputMode` `jsonTool`
- **THEN** the JSON schema instruction SHALL be injected into the system prompt and no `json` tool or forced tool choice SHALL be sent

#### Scenario: between_tools effort limit

- **WHEN** provider options set `reasoningConfig: {type: "between_tools", maxReasoningEffort: "max"}` for `global.anthropic.claude-sonnet-5-5`
- **THEN** `additionalModelRequestFields.thinking` SHALL equal `{type: "between_tools"}` and `output_config.effort` SHALL equal `high`
- **AND** an unsupported warning for `providerOptions.amazonBedrock.reasoningConfig.maxReasoningEffort` SHALL be emitted

#### Scenario: Claude Sonnet 5 keeps forced tool use

- **WHEN** `anthropic.claude-sonnet-5` is called with tool choice `required`
- **THEN** `toolConfig.toolChoice` SHALL be `any`
