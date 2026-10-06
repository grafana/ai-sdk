## ADDED Requirements

### Requirement: Model capabilities lookup
The system SHALL provide an unexported `getModelCapabilities` function that accepts a model ID string and returns a capabilities struct containing `maxOutputTokens` (int), `supportsAdaptiveThinking` (bool), and `isKnownModel` (bool).

The function SHALL use substring matching on the model ID, checked in specificity order:
- IDs containing `claude-sonnet-4-6` or `claude-opus-4-6`: 128000 max output, adaptive thinking supported, known
- IDs containing `claude-sonnet-4-5`, `claude-opus-4-5`, or `claude-haiku-4-5`: 64000 max output, adaptive thinking not supported, known
- IDs containing `claude-opus-4-1`: 32000 max output, adaptive thinking not supported, known
- IDs containing `claude-sonnet-4-` (catch-all for other sonnet 4.x): 64000 max output, adaptive thinking not supported, known
- IDs containing `claude-opus-4-` (catch-all for other opus 4.x): 32000 max output, adaptive thinking not supported, known
- IDs containing `claude-3-haiku`: 4096 max output, adaptive thinking not supported, known
- All other IDs: 4096 max output, adaptive thinking not supported, not known

#### Scenario: Known model claude-sonnet-4-6
- **WHEN** `getModelCapabilities` is called with model ID `claude-sonnet-4-6`
- **THEN** it SHALL return maxOutputTokens=128000, supportsAdaptiveThinking=true, isKnownModel=true

#### Scenario: Known model with date suffix
- **WHEN** `getModelCapabilities` is called with model ID `claude-sonnet-4-5@20250929`
- **THEN** it SHALL return maxOutputTokens=64000, supportsAdaptiveThinking=false, isKnownModel=true

#### Scenario: Known model claude-opus-4-1
- **WHEN** `getModelCapabilities` is called with model ID `claude-opus-4-1`
- **THEN** it SHALL return maxOutputTokens=32000, supportsAdaptiveThinking=false, isKnownModel=true

#### Scenario: Known model claude-3-haiku
- **WHEN** `getModelCapabilities` is called with model ID `claude-3-haiku`
- **THEN** it SHALL return maxOutputTokens=4096, supportsAdaptiveThinking=false, isKnownModel=true

#### Scenario: Unknown model
- **WHEN** `getModelCapabilities` is called with model ID `some-future-model`
- **THEN** it SHALL return maxOutputTokens=4096, supportsAdaptiveThinking=false, isKnownModel=false

### Requirement: Model-aware default max tokens
When `CallOptions.MaxOutputTokens` is nil, `buildParams` SHALL set `MaxTokens` to the model's `maxOutputTokens` from `getModelCapabilities` instead of the hardcoded 4096.

When `CallOptions.MaxOutputTokens` is set, `buildParams` SHALL use the user-provided value.

#### Scenario: No explicit max tokens on a Claude 4.6 model
- **WHEN** `buildParams` is called with model ID containing `claude-sonnet-4-6` and `MaxOutputTokens` is nil
- **THEN** `MaxTokens` SHALL be set to 128000

#### Scenario: No explicit max tokens on an unknown model
- **WHEN** `buildParams` is called with an unknown model ID and `MaxOutputTokens` is nil
- **THEN** `MaxTokens` SHALL be set to 4096

#### Scenario: User provides explicit max tokens
- **WHEN** `buildParams` is called with `MaxOutputTokens` set to 2048
- **THEN** `MaxTokens` SHALL be set to 2048 regardless of model capabilities

### Requirement: Default thinking budget
When thinking is enabled with type `enabled` but `budgetTokens` is not provided (zero value), `buildParams` SHALL default the budget to 1024 tokens and emit a compatibility warning with type `compatibility`, feature `extended thinking`, and details matching the upstream message.

#### Scenario: Thinking enabled without budget
- **WHEN** thinking is enabled with type `enabled` and no `budgetTokens` provided
- **THEN** `budgetTokens` SHALL be set to 1024 and a compatibility warning SHALL be emitted

### Requirement: Thinking budget adjustment
When thinking is enabled with type `enabled` and a `budgetTokens` value, `buildParams` SHALL add the thinking budget to `MaxTokens`.

When thinking is enabled with type `adaptive`, no budget adjustment SHALL be made (adaptive thinking does not have an explicit budget).

#### Scenario: Thinking enabled with budget
- **WHEN** thinking is enabled with `budgetTokens=10000` and base `MaxTokens` is 64000
- **THEN** `MaxTokens` SHALL be adjusted to 74000

#### Scenario: Thinking adaptive with no budget
- **WHEN** thinking is enabled with type `adaptive` and base `MaxTokens` is 64000
- **THEN** `MaxTokens` SHALL remain 64000

### Requirement: Max tokens clamping for known models
When the final `MaxTokens` (after thinking budget adjustment) exceeds the model's `maxOutputTokens` for a known model, `buildParams` SHALL clamp `MaxTokens` to `maxOutputTokens`.

If the user explicitly set `MaxOutputTokens` (not nil), a warning SHALL be emitted with type `unsupported`, feature `maxOutputTokens`, and details describing the clamping.

If the user did not set `MaxOutputTokens` (nil), clamping SHALL occur silently without a warning.

For unknown models (isKnownModel=false), no clamping SHALL occur.

#### Scenario: Clamping with user-provided max tokens and thinking budget
- **WHEN** model is `claude-sonnet-4-5` (max 64000), user sets `MaxOutputTokens=60000`, and thinking budget is 10000
- **THEN** `MaxTokens` SHALL be clamped to 64000 and a warning SHALL be emitted

#### Scenario: Clamping with default max tokens (no warning)
- **WHEN** model is `claude-3-haiku` (max 4096), `MaxOutputTokens` is nil, thinking budget is 10000
- **THEN** `MaxTokens` SHALL be clamped to 4096 with no warning emitted

#### Scenario: No clamping for unknown models
- **WHEN** model is unknown, user sets `MaxOutputTokens=200000`
- **THEN** `MaxTokens` SHALL remain 200000 with no clamping and no warning

#### Scenario: No clamping when within limits
- **WHEN** model is `claude-sonnet-4-6` (max 128000), user sets `MaxOutputTokens=50000`, no thinking budget
- **THEN** `MaxTokens` SHALL remain 50000 with no warning
