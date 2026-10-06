## ADDED Requirements

### Requirement: Model capability detection
The Anthropic module SHALL provide a `getModelCapabilities(modelID string)` function that returns `maxOutputTokens int`, `supportsAdaptiveThinking bool`, and `isKnownModel bool` based on substring matching of the model ID. The capabilities SHALL be:

| Model ID contains | maxOutputTokens | supportsAdaptiveThinking | isKnownModel |
|---|---|---|---|
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

#### Scenario: Sonnet 4-5 capabilities
- **WHEN** `getModelCapabilities("claude-sonnet-4-5-20250514")` is called
- **THEN** it SHALL return `maxOutputTokens: 64000`, `supportsAdaptiveThinking: false`, `isKnownModel: true`

#### Scenario: Unknown model capabilities
- **WHEN** `getModelCapabilities("some-future-model")` is called
- **THEN** it SHALL return `maxOutputTokens: 4096`, `supportsAdaptiveThinking: false`, `isKnownModel: false`

### Requirement: Reasoning resolution for adaptive-capable models
When `CallOptions.Reasoning` is set to a custom level (not nil, not `"provider-default"`) and the model supports adaptive thinking, the Anthropic provider SHALL set `thinking: adaptive` and map the reasoning level to an effort value:

| CallOptions.Reasoning | Anthropic effort |
|---|---|
| `"minimal"` | `"low"` (with compatibility warning) |
| `"low"` | `"low"` |
| `"medium"` | `"medium"` |
| `"high"` | `"high"` |
| `"xhigh"` | `"max"` (with compatibility warning) |

The provider SHALL emit a `compatibility` warning when the mapped effort value differs from the reasoning level name (i.e., `minimal` -> `low` and `xhigh` -> `max`).

#### Scenario: Reasoning high on adaptive model
- **WHEN** `CallOptions.Reasoning` is `"high"` and the model is `claude-sonnet-4-6`
- **THEN** the request SHALL contain `thinking.type` set to `"adaptive"` AND `output_config.effort` set to `"high"`
- **AND** no compatibility warning SHALL be emitted

#### Scenario: Reasoning xhigh on adaptive model
- **WHEN** `CallOptions.Reasoning` is `"xhigh"` and the model is `claude-sonnet-4-6`
- **THEN** the request SHALL contain `thinking.type` set to `"adaptive"` AND `output_config.effort` set to `"max"`
- **AND** a `compatibility` warning SHALL be emitted with feature `"reasoning"` and details indicating the mapping

#### Scenario: Reasoning minimal on adaptive model
- **WHEN** `CallOptions.Reasoning` is `"minimal"` and the model is `claude-opus-4-6`
- **THEN** the request SHALL contain `thinking.type` set to `"adaptive"` AND `output_config.effort` set to `"low"`
- **AND** a `compatibility` warning SHALL be emitted

### Requirement: Reasoning resolution for budget-based models
When `CallOptions.Reasoning` is set to a custom level (not nil, not `"provider-default"`, not `"none"`) and the model does NOT support adaptive thinking, the Anthropic provider SHALL set `thinking: enabled` with a computed `budgetTokens` value. The budget SHALL be calculated as `clamp(round(maxOutputTokens * percentage), 1024, maxOutputTokens)` where the percentages are:

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
When `CallOptions.Reasoning` is nil or `"provider-default"`, the Anthropic provider SHALL NOT set any thinking or effort configuration from the reasoning field. Existing provider-option-based configuration is unaffected.

#### Scenario: Reasoning nil
- **WHEN** `CallOptions.Reasoning` is nil
- **THEN** the reasoning mapping SHALL be skipped entirely

#### Scenario: Reasoning provider-default
- **WHEN** `CallOptions.Reasoning` is `"provider-default"`
- **THEN** the reasoning mapping SHALL be skipped entirely

### Requirement: Provider options take precedence over CallOptions.Reasoning
When EITHER `ProviderOptions["anthropic"]["thinking"]` OR `ProviderOptions["anthropic"]["effort"]` is set, the reasoning mapping SHALL be skipped entirely. Provider-specific options always win.

#### Scenario: Provider thinking already set
- **WHEN** `CallOptions.Reasoning` is `"high"` and `ProviderOptions["anthropic"]` contains `{"thinking":{"type":"enabled","budgetTokens":5000}}`
- **THEN** the reasoning mapping SHALL be skipped, and the provider-option thinking config SHALL be used

#### Scenario: Provider effort already set
- **WHEN** `CallOptions.Reasoning` is `"low"` and `ProviderOptions["anthropic"]` contains `{"effort":"max"}`
- **THEN** the reasoning mapping SHALL be skipped, and the provider-option effort SHALL be used

#### Scenario: Both provider options and reasoning set
- **WHEN** `CallOptions.Reasoning` is `"medium"` and `ProviderOptions["anthropic"]` contains `{"thinking":{"type":"adaptive"}, "effort":"high"}`
- **THEN** the reasoning mapping SHALL be skipped entirely

#### Scenario: Neither provider option set
- **WHEN** `CallOptions.Reasoning` is `"medium"` and `ProviderOptions["anthropic"]` does not contain `thinking` or `effort`
- **THEN** the reasoning mapping SHALL be applied

### Requirement: Reasoning effort beta header
The Anthropic provider SHALL append the `effort-2025-11-24` beta header when the reasoning mapping results in an effort value being set (adaptive path only). The thinking beta header `interleaved-thinking-2025-05-14` SHALL be appended when the reasoning mapping results in thinking being enabled or adaptive.

#### Scenario: Beta headers from adaptive reasoning
- **WHEN** `CallOptions.Reasoning` is `"high"` on an adaptive-capable model
- **THEN** the request betas SHALL include both `effort-2025-11-24` and `interleaved-thinking-2025-05-14`

#### Scenario: Beta headers from budget reasoning
- **WHEN** `CallOptions.Reasoning` is `"high"` on a budget-based model
- **THEN** the request betas SHALL include `interleaved-thinking-2025-05-14` but NOT `effort-2025-11-24`

#### Scenario: No beta headers for none
- **WHEN** `CallOptions.Reasoning` is `"none"`
- **THEN** no thinking or effort beta headers SHALL be added from the reasoning mapping
