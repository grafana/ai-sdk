## MODIFIED Requirements

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
