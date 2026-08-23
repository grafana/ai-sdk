## Purpose
Defines the Anthropic provider's effort-level configuration, model capability detection, and reasoning-to-thinking mapping.
## Requirements
### Requirement: Effort level provider option
The Anthropic provider SHALL accept an `effort` field in `AnthropicOptions` with values `low`, `medium`, `high`, `xhigh`, or `max`. When set, the provider SHALL include `output_config.effort` in the Anthropic API request body with the specified value.

#### Scenario: Effort level passed through to API request
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"effort":"high"}`
- **THEN** the built request params SHALL contain `output_config.effort` set to `"high"`

#### Scenario: Xhigh effort level passed through to API request
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"effort":"xhigh"}`
- **THEN** the built request params SHALL contain `output_config.effort` set to `"xhigh"`

#### Scenario: Effort level omitted when not set
- **WHEN** caller does not set an `effort` field in provider options
- **THEN** the built request params SHALL NOT contain an `output_config` object (unless set by other features)

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

### Requirement: Model capability detection
The `providers/anthropic` module SHALL provide a `getModelCapabilities(modelID string)` function that returns `maxOutputTokens int`, `supportsAdaptiveThinking bool`, and `isKnownModel bool` based on substring matching of the model ID. The capabilities SHALL be:

| Model ID contains | maxOutputTokens | supportsAdaptiveThinking | isKnownModel |
|---|---|---|---|
| `claude-opus-4-7` | 128000 | true | true |
| `claude-sonnet-4-6` or `claude-opus-4-6` | 128000 | true | true |
| `claude-sonnet-4-5`, `claude-opus-4-5`, or `claude-haiku-4-5` | 64000 | false | true |
| `claude-opus-4-1` | 32000 | false | true |
| Other `claude-sonnet-4-` | 64000 | false | true |
| Other `claude-opus-4-` | 32000 | false | true |
| `claude-3-haiku` | 4096 | false | true |
| Unknown | 4096 | false | false |

The matching order SHALL be specific-first (e.g., `claude-sonnet-4-6` checked before `claude-sonnet-4-`).

#### Scenario: Sonnet 4-6 capabilities
- **WHEN** `getModelCapabilities("claude-sonnet-4-6-20260101")` is called
- **THEN** it SHALL return `maxOutputTokens: 128000`, `supportsAdaptiveThinking: true`, `isKnownModel: true`

#### Scenario: Opus 4-7 capabilities
- **WHEN** `getModelCapabilities("claude-opus-4-7")` is called
- **THEN** it SHALL return `maxOutputTokens: 128000`, `supportsAdaptiveThinking: true`, `isKnownModel: true`

#### Scenario: Sonnet 4-5 capabilities
- **WHEN** `getModelCapabilities("claude-sonnet-4-5-20250514")` is called
- **THEN** it SHALL return `maxOutputTokens: 64000`, `supportsAdaptiveThinking: false`, `isKnownModel: true`

#### Scenario: Unknown model capabilities
- **WHEN** `getModelCapabilities("some-future-model")` is called
- **THEN** it SHALL return `maxOutputTokens: 4096`, `supportsAdaptiveThinking: false`, `isKnownModel: false`

### Requirement: Reasoning resolution for adaptive-capable models
When `CallOptions.Reasoning` is a non-zero operational level other than `ReasoningNone` and the model supports adaptive thinking, the Anthropic provider SHALL set `thinking: adaptive` and map the reasoning level to an effort value:

| CallOptions.Reasoning | Anthropic effort |
|---|---|
| `"minimal"` | `"low"` (with compatibility warning) |
| `"low"` | `"low"` |
| `"medium"` | `"medium"` |
| `"high"` | `"high"` |
| `"xhigh"` | `"max"` (with compatibility warning), or `"xhigh"` for models that support xhigh effort |

The provider SHALL emit a `compatibility` warning when the mapped effort value differs from the reasoning level name (i.e., `minimal` -> `low` and `xhigh` -> `max`). No compatibility warning SHALL be emitted when `xhigh` maps directly to `xhigh`.

#### Scenario: Reasoning high on adaptive model
- **WHEN** `CallOptions.Reasoning` is `"high"` and the model is `claude-sonnet-4-6`
- **THEN** the request SHALL contain `thinking.type` set to `"adaptive"` AND `output_config.effort` set to `"high"`
- **AND** no compatibility warning SHALL be emitted

#### Scenario: Reasoning xhigh on adaptive model
- **WHEN** `CallOptions.Reasoning` is `"xhigh"` and the model is `claude-sonnet-4-6`
- **THEN** the request SHALL contain `thinking.type` set to `"adaptive"` AND `output_config.effort` set to `"max"`
- **AND** a `compatibility` warning SHALL be emitted with feature `"reasoning"` and details indicating the mapping

#### Scenario: Reasoning xhigh on xhigh-capable adaptive model
- **WHEN** `CallOptions.Reasoning` is `"xhigh"` and the model is `claude-opus-4-7`
- **THEN** the request SHALL contain `thinking.type` set to `"adaptive"` AND `output_config.effort` set to `"xhigh"`
- **AND** no compatibility warning SHALL be emitted

#### Scenario: Reasoning minimal on adaptive model
- **WHEN** `CallOptions.Reasoning` is `"minimal"` and the model is `claude-opus-4-6`
- **THEN** the request SHALL contain `thinking.type` set to `"adaptive"` AND `output_config.effort` set to `"low"`
- **AND** a `compatibility` warning SHALL be emitted

### Requirement: Reasoning resolution for budget-based models
When `CallOptions.Reasoning` is a non-zero operational level other than `ReasoningNone` and the model does NOT support adaptive thinking, the Anthropic provider SHALL set `thinking: enabled` with a computed `budgetTokens` value. The budget SHALL be calculated as `clamp(round(maxOutputTokens * percentage), 1024, maxOutputTokens)` where the percentages are:

| CallOptions.Reasoning | Percentage |
|---|---|
| `"minimal"` | 2% |
| `"low"` | 10% |
| `"medium"` | 30% |
| `"high"` | 60% |
| `"xhigh"` | 90% |

No `output_config.effort` SHALL be set for budget-based models.

#### Scenario: Reasoning medium on Sonnet 4-5
- **WHEN** `CallOptions.Reasoning` is `"medium"` and the model is `claude-sonnet-4-5` (maxOutputTokens 64000)
- **THEN** the request SHALL contain `thinking.type` set to `"enabled"` with `budget_tokens` set to `19200` (round(64000 * 0.3))
- **AND** no `output_config.effort` SHALL be present

#### Scenario: Reasoning minimal on Sonnet 4-5
- **WHEN** `CallOptions.Reasoning` is `"minimal"` and the model is `claude-sonnet-4-5` (maxOutputTokens 64000)
- **THEN** the request SHALL contain `thinking.type` set to `"enabled"` with `budget_tokens` set to `1280` (round(64000 * 0.02))

#### Scenario: Reasoning xhigh on Sonnet 4-5
- **WHEN** `CallOptions.Reasoning` is `"xhigh"` and the model is `claude-sonnet-4-5` (maxOutputTokens 64000)
- **THEN** the request SHALL contain `thinking.type` set to `"enabled"` with `budget_tokens` set to `57600` (round(64000 * 0.9))

#### Scenario: Budget clamped to minimum
- **WHEN** `CallOptions.Reasoning` is `"minimal"` and a model has `maxOutputTokens: 4096`
- **THEN** `budget_tokens` SHALL be `1024` (clamp minimum), not `82` (round(4096 * 0.02))

### Requirement: Reasoning none disables thinking
When `CallOptions.Reasoning` is `"none"`, the Anthropic provider SHALL set `thinking: disabled`. No `output_config.effort` SHALL be set.

#### Scenario: Reasoning none on any model
- **WHEN** `CallOptions.Reasoning` is `"none"`
- **THEN** the request SHALL contain `thinking.type` set to `"disabled"`
- **AND** no `output_config.effort` SHALL be present
- **AND** no effort beta header SHALL be added

### Requirement: Reasoning nil and provider-default are no-ops
When `CallOptions.Reasoning` is the zero-valued `ReasoningProviderDefault`, the Anthropic provider SHALL NOT set any thinking or effort configuration from the reasoning field. A strict wire adapter receiving explicit `"provider-default"` SHALL normalize it to the same zero value. Existing provider-option-based configuration is unaffected.

#### Scenario: Reasoning zero value
- **WHEN** `CallOptions.Reasoning` is not explicitly set
- **THEN** the reasoning mapping SHALL be skipped entirely

#### Scenario: Explicit wire provider-default
- **WHEN** ProviderWire maps explicit `"provider-default"` into `CallOptions.Reasoning`
- **THEN** the Anthropic reasoning mapping SHALL be skipped entirely

#### Scenario: Provider options remain authoritative
- **WHEN** zero-valued reasoning is combined with Anthropic `thinking` or `effort` provider options
- **THEN** those provider options SHALL retain their existing behavior

### Requirement: Provider options take precedence over CallOptions.Reasoning
The provider option `effort` SHALL gate the reasoning mapping entirely: when `ProviderOptions["anthropic"]["effort"]` is set, no part of the reasoning mapping SHALL be applied. The provider option `thinking` SHALL only gate the thinking portion of the mapping: when `ProviderOptions["anthropic"]["thinking"]` is set without `effort`, the provider-supplied thinking config SHALL be preserved AND the effort SHALL still be derived from the top-level `CallOptions.Reasoning`. When `ProviderOptions["anthropic"]["thinking"]` is set to `{"type":"disabled"}`, the derived effort SHALL NOT be applied (mirrors upstream `anthropic-language-model.ts:406-411`). This mirrors upstream's gating logic where `anthropicOptions?.effort == null` is the sole condition for invoking the reasoning resolver, while the derived `thinking` field is only assigned when `anthropicOptions.thinking == null`.

#### Scenario: Provider thinking only (effort unset) still derives effort
- **WHEN** `CallOptions.Reasoning` is `"high"` and `ProviderOptions["anthropic"]` contains `{"thinking":{"type":"enabled","budgetTokens":5000}}`
- **THEN** the provider-supplied thinking config SHALL be used (not overridden by the reasoning-derived thinking)
- **AND** `output_config.effort` SHALL be derived from `CallOptions.Reasoning` and set to `"high"`

#### Scenario: Provider effort already set skips entire mapping
- **WHEN** `CallOptions.Reasoning` is `"low"` and `ProviderOptions["anthropic"]` contains `{"effort":"max"}`
- **THEN** the reasoning mapping SHALL be skipped, and the provider-option effort `"max"` SHALL be used
- **AND** no thinking config SHALL be derived from reasoning

#### Scenario: Provider thinking=disabled blocks effort derivation
- **WHEN** `CallOptions.Reasoning` is `"high"` and `ProviderOptions["anthropic"]` contains `{"thinking":{"type":"disabled"}}`
- **THEN** the provider-supplied thinking config SHALL be used (`type: "disabled"`)
- **AND** `output_config.effort` SHALL NOT be set from reasoning

#### Scenario: Both provider options and reasoning set
- **WHEN** `CallOptions.Reasoning` is `"medium"` and `ProviderOptions["anthropic"]` contains `{"thinking":{"type":"adaptive"}, "effort":"high"}`
- **THEN** the reasoning mapping SHALL be skipped entirely (the `effort` gate fires)

#### Scenario: Neither provider option set
- **WHEN** `CallOptions.Reasoning` is `"medium"` and `ProviderOptions["anthropic"]` does not contain `thinking` or `effort`
- **THEN** the reasoning mapping SHALL be applied in full (both thinking config and effort derived)

### Requirement: No effort beta header is appended
The Anthropic provider SHALL NOT append any beta header related to the `effort` parameter when `output_config.effort` is set on the request. This applies to both the provider-options path (`AnthropicOptions.Effort`) and the reasoning-mapping path (`CallOptions.Reasoning` on adaptive-thinking-capable models). The `output_config.effort` request body field alone drives the feature.

#### Scenario: No effort beta when AnthropicOptions.Effort is set
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"effort":"high"}`
- **THEN** the built request params SHALL contain `output_config.effort` set to `"high"`
- **AND** the request betas SHALL NOT include `effort-2025-11-24`

#### Scenario: No effort beta when reasoning maps to effort on adaptive model
- **WHEN** `CallOptions.Reasoning` is `"high"` and the model is `claude-sonnet-4-6`
- **THEN** the request SHALL contain `thinking.type` set to `"adaptive"` AND `output_config.effort` set to `"high"`
- **AND** the request betas SHALL include `interleaved-thinking-2025-05-14`
- **AND** the request betas SHALL NOT include `effort-2025-11-24`

#### Scenario: No effort beta when reasoning xhigh maps to xhigh on Opus 4-7
- **WHEN** `CallOptions.Reasoning` is `"xhigh"` and the model is `claude-opus-4-7`
- **THEN** the request SHALL contain `output_config.effort` set to `"xhigh"`
- **AND** the request betas SHALL NOT include `effort-2025-11-24`

#### Scenario: No effort beta when reasoning is budget-based
- **WHEN** `CallOptions.Reasoning` is `"high"` on a budget-based model (e.g., `claude-sonnet-4-5`)
- **THEN** the request betas SHALL include `interleaved-thinking-2025-05-14`
- **AND** the request betas SHALL NOT include `effort-2025-11-24`
- **AND** no `output_config.effort` SHALL be present

### Requirement: Adaptive thinking display
The Anthropic provider SHALL accept a `display` field in `ThinkingConfig` with values `"summarized"` or `"omitted"`. When set and the configured `type` is `"adaptive"`, the provider SHALL include `thinking.display` in the Anthropic API request body with the specified value. The `display` field SHALL be ignored when `type` is `"enabled"` or `"disabled"`.

#### Scenario: Display set on adaptive thinking
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"thinking":{"type":"adaptive","display":"summarized"}}`
- **THEN** the built request params SHALL contain `thinking.type` set to `"adaptive"` and `thinking.display` set to `"summarized"`

#### Scenario: Display omitted on adaptive thinking
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"thinking":{"type":"adaptive"}}` (no display)
- **THEN** the built request params SHALL contain `thinking.type` set to `"adaptive"` and SHALL NOT contain `thinking.display`

#### Scenario: Display ignored on enabled thinking
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"thinking":{"type":"enabled","budgetTokens":5000,"display":"omitted"}}`
- **THEN** the built request params SHALL NOT contain `thinking.display`

### Requirement: Task budget provider option
The Anthropic provider SHALL accept a `taskBudget` field in `AnthropicOptions` containing `type` (literal `"tokens"`), `total` (int64), and optional `remaining` (int64). When set, the provider SHALL include `output_config.task_budget` in the Anthropic API request body with `type`, `total`, and (when provided) `remaining`. The provider SHALL append the `task-budgets-2026-03-13` beta header when `taskBudget` is set.

#### Scenario: Task budget with total only
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"taskBudget":{"type":"tokens","total":50000}}`
- **THEN** the built request params SHALL contain `output_config.task_budget` with `type` set to `"tokens"` and `total` set to `50000`
- **AND** `output_config.task_budget.remaining` SHALL NOT be present
- **AND** the request betas SHALL include `task-budgets-2026-03-13`

#### Scenario: Task budget with remaining
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"taskBudget":{"type":"tokens","total":50000,"remaining":30000}}`
- **THEN** the built request params SHALL contain `output_config.task_budget.remaining` set to `30000`

#### Scenario: Task budget beta absent when not set
- **WHEN** caller does not set `taskBudget` in provider options
- **THEN** the request betas SHALL NOT include `task-budgets-2026-03-13`
- **AND** the request SHALL NOT contain `output_config.task_budget`

### Requirement: Task budget validation
The Anthropic provider SHALL validate `taskBudget` against the upstream Zod schema constraints before sending the request:

- `type`: when non-empty, SHALL equal `"tokens"`. An empty `type` SHALL be treated as `"tokens"` (Go zero-value convention).
- `total`: SHALL be at least `20000`.
- `remaining`: when present, SHALL be `>= 0`.

When any constraint is violated, the provider SHALL emit a single `other` warning with feature `taskBudget` describing the violation and SHALL NOT include `output_config.task_budget` or the `task-budgets-2026-03-13` beta in the request.

#### Scenario: Unsupported task budget type
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"taskBudget":{"type":"requests","total":50000}}`
- **THEN** an `other` warning SHALL be emitted with feature `taskBudget` and a message indicating only `"tokens"` is supported
- **AND** the request SHALL NOT contain `output_config.task_budget`
- **AND** the request betas SHALL NOT include `task-budgets-2026-03-13`

#### Scenario: Task budget total below minimum
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"taskBudget":{"type":"tokens","total":1000}}`
- **THEN** an `other` warning SHALL be emitted with feature `taskBudget` indicating `total` must be at least `20000`
- **AND** the request SHALL NOT contain `output_config.task_budget`

#### Scenario: Task budget negative remaining
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"taskBudget":{"type":"tokens","total":50000,"remaining":-1}}`
- **THEN** an `other` warning SHALL be emitted with feature `taskBudget` indicating `remaining` must be `>= 0`
- **AND** the request SHALL NOT contain `output_config.task_budget`

#### Scenario: Empty type defaults to tokens
- **WHEN** caller sets `ProviderOptions["anthropic"]` with `{"taskBudget":{"total":50000}}` (no `type`)
- **THEN** the budget SHALL be applied with `type` marshaled as `"tokens"`
- **AND** no validation warning SHALL be emitted
