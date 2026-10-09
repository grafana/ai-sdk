## Purpose

Define Anthropic model capability lookup and the model-aware validation, defaults, thinking-budget adjustment, and output-token clamping derived from those capabilities.

## Requirements

### Requirement: Model capabilities lookup

The system SHALL provide unexported `getModelCapabilities(modelID string)` returning model metadata without rewriting the request ID. Specific family checks SHALL precede generic Claude 4 checks, and Sonnet 5.5 SHALL precede Sonnet 5. Classification SHALL use registered baseline families plus Sonnet 5.5 ported ahead from `@ai-sdk/anthropic` 4.0.67. Boolean capabilities not explicitly enabled in a classification SHALL be false; bare Sonnet/Opus 4 SHALL remain unknown-Claude.

#### Scenario: Known model claude-opus-4-7
- **WHEN** `getModelCapabilities` is called with model ID `claude-opus-4-7`
- **THEN** it SHALL return maxOutputTokens=128000, supportsAdaptiveThinking=true, supportsStructuredOutput=true, rejectsSamplingParams=true, supportsXHighEffort=true, isKnownModel=true

#### Scenario: Known model claude-sonnet-5-5
- **WHEN** `getModelCapabilities` is called with `claude-sonnet-5-5`, `anthropic.claude-sonnet-5-5` or `global.anthropic.claude-sonnet-5-5`
- **THEN** it SHALL return maxOutputTokens=128000, supportsAdaptiveThinking=true, supportsStructuredOutput=true, rejectsSamplingParams=true, supportsXHighEffort=true, rejectsThinkingDisabledAboveHighEffort=true, rejectsThinkingDisabled=true, rejectsForcedToolUse=true, supportsBetweenToolsThinking=true, isKnownModel=true

#### Scenario: Claude Sonnet 5 keeps disabled thinking and forced tool use
- **WHEN** `getModelCapabilities` is called with `claude-sonnet-5`
- **THEN** rejectsThinkingDisabled, rejectsForcedToolUse and supportsBetweenToolsThinking SHALL be false

#### Scenario: Known model claude-sonnet-4-6
- **WHEN** `getModelCapabilities` is called with model ID `claude-sonnet-4-6`
- **THEN** it SHALL return maxOutputTokens=128000, supportsAdaptiveThinking=true, supportsStructuredOutput=true, isKnownModel=true

#### Scenario: Known model with date suffix
- **WHEN** `getModelCapabilities` is called with model ID `claude-sonnet-4-5@20250929`
- **THEN** it SHALL return maxOutputTokens=64000, supportsAdaptiveThinking=false, supportsStructuredOutput=true, isKnownModel=true

#### Scenario: Known model claude-opus-4-1
- **WHEN** `getModelCapabilities` is called with model ID `claude-opus-4-1`
- **THEN** it SHALL return maxOutputTokens=32000, supportsAdaptiveThinking=false, supportsStructuredOutput=true, isKnownModel=true

#### Scenario: Known model claude-3-haiku
- **WHEN** `getModelCapabilities` is called with model ID `claude-3-haiku`
- **THEN** it SHALL return maxOutputTokens=4096, supportsAdaptiveThinking=false, supportsStructuredOutput=false, isKnownModel=true

#### Scenario: Older sonnet model
- **WHEN** `getModelCapabilities` is called with model ID `claude-sonnet-4-0`
- **THEN** it SHALL return maxOutputTokens=64000, supportsAdaptiveThinking=false, supportsStructuredOutput=false, isKnownModel=true

#### Scenario: Direct date and resolved Vertex date for the same base model
- **WHEN** `getModelCapabilities` is called with `claude-sonnet-4-20250514` or `claude-sonnet-4@20250514`
- **THEN** both SHALL return maxOutputTokens=64000, supportsAdaptiveThinking=false, supportsStructuredOutput=false, rejectsSamplingParams=false, isKnownModel=true

#### Scenario: Opus base model with Vertex date
- **WHEN** `getModelCapabilities` is called with `claude-opus-4@20250514`
- **THEN** it SHALL return maxOutputTokens=32000, supportsAdaptiveThinking=false, supportsStructuredOutput=false, rejectsSamplingParams=false, isKnownModel=true

#### Scenario: Specific Claude 4 and Claude 5 precede base-family checks
- **WHEN** `getModelCapabilities` is called with `claude-sonnet-4-5-20250929`, `claude-opus-4-6`, or `claude-sonnet-5`
- **THEN** each SHALL retain its specific row's capabilities rather than the generic Claude 4 base-family defaults

#### Scenario: Unknown Claude base name lacks the boundary
- **WHEN** `getModelCapabilities` is called with `claude-sonnet-4`
- **THEN** it SHALL remain unknown with maxOutputTokens=128000, supportsAdaptiveThinking=true and rejectsSamplingParams=true

#### Scenario: Unknown model
- **WHEN** `getModelCapabilities` is called with model ID `some-future-model`
- **THEN** it SHALL return maxOutputTokens=4096, supportsAdaptiveThinking=false, supportsStructuredOutput=false, isKnownModel=false

#### Scenario: Sonnet 5.5 capability classification

- **WHEN** an ID contains `claude-sonnet-5-5`
- **THEN** metadata SHALL report 128000 max output, adaptive and structured output, sampling rejected, xhigh effort supported, disabled thinking rejected above high effort, disabled and budget thinking rejected, forced tool use rejected, `between_tools` thinking supported, known; boolean capabilities not enabled here SHALL be false

#### Scenario: Opus 5 capability classification

- **WHEN** an ID contains `claude-opus-5`
- **THEN** metadata SHALL report 128000 max output, adaptive and structured output, sampling rejected, xhigh effort supported, disabled thinking rejected above high effort, known; boolean capabilities not enabled here SHALL be false

#### Scenario: Newest Claude capability classification

- **WHEN** an ID contains `claude-opus-4-8`, `claude-opus-4-7`, `claude-fable-5`, or `claude-sonnet-5` but does not match Sonnet 5.5
- **THEN** metadata SHALL report 128000 max output, adaptive and structured output, sampling rejected, xhigh effort supported, known; boolean capabilities not enabled here SHALL be false

#### Scenario: Claude 4.6 capability classification

- **WHEN** an ID contains `claude-sonnet-4-6` or `claude-opus-4-6`
- **THEN** metadata SHALL report 128000 max output, adaptive and structured output, known; sampling not rejected; boolean capabilities not enabled here SHALL be false

#### Scenario: Claude 4.5 capability classification

- **WHEN** an ID contains `claude-sonnet-4-5`, `claude-opus-4-5`, or `claude-haiku-4-5`
- **THEN** metadata SHALL report 64000 max output, structured output, known; no adaptive thinking or sampling rejection; boolean capabilities not enabled here SHALL be false

#### Scenario: Opus 4.1 capability classification

- **WHEN** an ID contains `claude-opus-4-1`
- **THEN** metadata SHALL report 32000 max output, structured output, known; no adaptive thinking or sampling rejection; boolean capabilities not enabled here SHALL be false

#### Scenario: Base Sonnet 4 capability classification

- **WHEN** an ID matches `claude-sonnet-4` immediately followed by `-` or `@` and no more-specific family
- **THEN** metadata SHALL report 64000 max output, known; no adaptive thinking, structured output or sampling rejection; boolean capabilities not enabled here SHALL be false

#### Scenario: Base Opus 4 capability classification

- **WHEN** an ID matches `claude-opus-4` immediately followed by `-` or `@` and no more-specific family
- **THEN** metadata SHALL report 32000 max output, known; no adaptive thinking, structured output or sampling rejection; boolean capabilities not enabled here SHALL be false

#### Scenario: Haiku 3 capability classification

- **WHEN** an ID contains `claude-3-haiku`
- **THEN** metadata SHALL report 4096 max output, known; no adaptive thinking, structured output or sampling rejection; boolean capabilities not enabled here SHALL be false

#### Scenario: Legacy Claude capability classification

- **WHEN** an ID belongs to legacy Claude 2/3/instant families and no more-specific family
- **THEN** metadata SHALL report 4096 max output and not known; boolean capabilities not enabled here SHALL be false

#### Scenario: Unknown Claude capability classification

- **WHEN** an ID contains `claude-` but matches no known or legacy family, including bare `claude-sonnet-4` and `claude-opus-4`
- **THEN** metadata SHALL report 128000 max output, adaptive and structured output, sampling rejected, xhigh effort supported, disabled thinking rejected above high effort, not known; boolean capabilities not enabled here SHALL be false

#### Scenario: Non-Claude capability classification

- **WHEN** an ID matches none of the Claude classifications
- **THEN** metadata SHALL report 4096 max output and not known; boolean capabilities not enabled here SHALL be false

### Requirement: Sampling parameter rejection for unsupported models
When model capabilities indicate `rejectsSamplingParams=true`, `buildParams` SHALL clear `Temperature`, `TopP`, and `TopK` from the Anthropic request and emit unsupported warnings for each cleared field.

#### Scenario: Sampling parameters rejected for claude-opus-4-7
- **WHEN** `buildParams` is called with model ID `claude-opus-4-7` and `Temperature`, `TopP`, and `TopK` are set
- **THEN** the Anthropic request SHALL omit `temperature`, `top_p`, and `top_k`
- **AND** warnings SHALL be emitted for `temperature`, `topP`, and `topK`

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

When final `MaxTokens` after thinking-budget adjustment exceeds a known model's `maxOutputTokens`, `buildParams` SHALL clamp to that maximum. If `MaxOutputTokens` is non-nil, clamping SHALL emit an `unsupported` warning for `maxOutputTokens` describing the clamp; nil SHALL clamp silently. Unknown models (`isKnownModel=false`) SHALL NOT clamp.

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

### Requirement: Dated Claude model request capabilities
For both direct and Vertex request paths, the Anthropic provider SHALL use classified capabilities to select default/clamped max tokens, thinking support, and sampling warnings, without altering the direct ID supplied by the caller or the ID produced by `ResolveVertexModelID`. The existing explicit max-token override and warning behavior SHALL remain in force.

#### Scenario: Dated base Sonnet/Opus 4 request on both providers
- **WHEN** a direct request uses `claude-sonnet-4-20250514` or `claude-opus-4-20250514`, or a Vertex request resolves those IDs to `claude-sonnet-4@20250514` or `claude-opus-4@20250514`, with no explicit max output tokens and a top-level reasoning hint
- **THEN** request max tokens SHALL default to 64000 for Sonnet and 32000 for Opus, adaptive thinking SHALL not be selected for these models, and each transport SHALL retain its own model-ID form

#### Scenario: Sampling on dated base Claude 4 models without thinking
- **WHEN** direct or Vertex requests for those dated base models specify sampling parameters without active thinking
- **THEN** those parameters SHALL follow the known non-rejecting model behavior; no sampling warning SHALL be emitted solely because the model is treated as unknown

#### Scenario: Specific dated Claude 4 and Claude 5 requests
- **WHEN** direct or Vertex requests use dated or undated 4.5, 4.6, 4.7, 4.8 or 5 model IDs for which specific capability rows apply
- **THEN** the request SHALL follow the specific max-token, thinking and sampling rules and warnings for that row instead of the generic Sonnet/Opus 4 row

#### Scenario: Unknown model request fallback
- **WHEN** an unrecognized Claude model ID or a non-Claude model ID is requested without explicit max tokens
- **THEN** the request SHALL keep its existing unknown-model default max tokens and compatibility warning; it SHALL not be classified as a known dated Claude 4 model

### Requirement: Thinking normalization for models that reject disabled thinking

For `rejectsThinkingDisabled` models, explicit thinking `disabled` SHALL become `between_tools` if supported, else be removed; budget-based `enabled` SHALL become budgetless `adaptive`. As in `@ai-sdk/anthropic` 4.0.67, each rewrite SHALL warn `unsupported` with feature `providerOptions.anthropic.thinking` and model ID in details, before disabled-above-high and between-tools effort caps.

#### Scenario: Explicit disabled thinking on Sonnet 5.5
- **WHEN** `claude-sonnet-5-5` is called with `thinking: {type: "disabled"}`
- **THEN** the request SHALL contain `thinking: {type: "between_tools"}`
- **AND** an unsupported warning with feature `providerOptions.anthropic.thinking` SHALL say that thinking cannot be disabled and that `between_tools` is used instead

#### Scenario: Budget-based thinking on Sonnet 5.5
- **WHEN** `claude-sonnet-5-5` is called with `thinking: {type: "enabled", budgetTokens: 5000}`
- **THEN** the request SHALL contain `thinking: {type: "adaptive"}` without `budget_tokens`, and `MaxTokens` SHALL NOT include the budget
- **AND** an unsupported warning with feature `providerOptions.anthropic.thinking` SHALL be emitted

#### Scenario: Other models keep their thinking setting
- **WHEN** `claude-sonnet-5` is called with `thinking: {type: "disabled"}`
- **THEN** the request SHALL contain `thinking: {type: "disabled"}` and no thinking rewrite warning

### Requirement: Between-tools thinking effort limit
`between_tools` thinking SHALL be sent as `{type: "between_tools"}` with no `display` or `budget_tokens`, and SHALL count as active thinking for sampling-parameter removal. When the effective effort is `xhigh` or `max`, the provider SHALL lower it to `high` and emit an `unsupported` warning with feature `providerOptions.anthropic.effort`, as `@ai-sdk/anthropic` 4.0.67 does, because the API rejects `between_tools` above `high`.

#### Scenario: between_tools at xhigh effort
- **WHEN** `claude-sonnet-5-5` is called with `thinking: {type: "between_tools"}` and `effort: "xhigh"`
- **THEN** the request SHALL contain `output_config.effort` set to `high`
- **AND** an unsupported warning with feature `providerOptions.anthropic.effort` SHALL be emitted

#### Scenario: Disabled thinking at max effort
- **WHEN** `claude-sonnet-5-5` is called with `thinking: {type: "disabled"}` and `effort: "max"`
- **THEN** the request SHALL contain `thinking: {type: "between_tools"}` and `output_config.effort` set to `high`

#### Scenario: Display is not sent with between_tools
- **WHEN** `claude-sonnet-5-5` is called with `thinking: {type: "between_tools", display: "summarized"}`
- **THEN** the request SHALL contain `thinking: {type: "between_tools"}` only

#### Scenario: Sampling parameters dropped with between_tools
- **WHEN** `claude-sonnet-5-5` is called with root reasoning `none` and a temperature
- **THEN** the request SHALL omit `temperature` and emit an unsupported `temperature` warning

### Requirement: Forced tool choice fallback

For `rejectsForcedToolUse` models without a JSON response tool, required choice SHALL become auto; named choice SHALL become auto with only that tool, matched after name mapping to keep renamed provider tools. As in `@ai-sdk/anthropic` 4.0.67 `prepareTools`, each fallback SHALL warn unsupported `toolChoice`, telling callers to instruct the model in the prompt and verify the call.

#### Scenario: Required tool choice on Sonnet 5.5
- **WHEN** `claude-sonnet-5-5` is called with two function tools and tool choice `required`
- **THEN** the request SHALL contain `tool_choice: {type: "auto"}` and both tools
- **AND** one unsupported `toolChoice` warning SHALL be emitted

#### Scenario: Named tool choice on Sonnet 5.5
- **WHEN** `claude-sonnet-5-5` is called with tools `weather` and `search` and tool choice `tool` named `search`
- **THEN** the request SHALL contain `tool_choice: {type: "auto"}` and only the `search` tool
- **AND** one unsupported `toolChoice` warning SHALL name `search`

#### Scenario: Forced caller choice with the JSON response tool
- **WHEN** `claude-sonnet-5-5` is called through a transport without native structured output, with tools, a JSON schema response and tool choice `required` or `tool`
- **THEN** every caller tool and the `json` tool SHALL be sent with `tool_choice: {type: "auto", disable_parallel_tool_use: true}`
- **AND** exactly one unsupported `toolChoice` warning, for `required`, SHALL be emitted

#### Scenario: Sonnet 5 keeps forced tool use
- **WHEN** `claude-sonnet-5` is called with tool choice `required`
- **THEN** the request SHALL contain `tool_choice: {type: "any"}`

### Requirement: Anthropic capability metadata fields

Capability metadata SHALL include `maxOutputTokens`, `supportsAdaptiveThinking`, `supportsStructuredOutput`, `rejectsSamplingParams`, `supportsXHighEffort`, `rejectsThinkingDisabledAboveHighEffort`, `rejectsThinkingDisabled`, `rejectsForcedToolUse`, `supportsBetweenToolsThinking` and `isKnownModel`.

#### Scenario: Anthropic capability metadata fields

- **WHEN** capabilities are looked up for `claude-sonnet-5-5` and `some-future-model`
- **THEN** each result SHALL report every listed field; Sonnet 5.5 SHALL enable its classified boolean capabilities, while the non-Claude unknown model SHALL report all booleans false and maxOutputTokens=4096

### Requirement: Anthropic JSON response tool replaces caller choice

With a JSON response tool, that tool SHALL replace caller choice as specified by `anthropic-structured-output`; caller choice SHALL NOT filter tools or add its own warning.

#### Scenario: Anthropic JSON response tool replaces caller choice

- **WHEN** Sonnet 5.5 uses a JSON fallback tool with several caller tools and a named choice
- **THEN** all tools SHALL survive and only the fallback-required choice SHALL produce the unsupported toolChoice warning
