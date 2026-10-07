## Purpose

Define Anthropic model capability lookup and the model-aware validation, defaults, thinking-budget adjustment, and output-token clamping derived from those capabilities.

## Requirements

### Requirement: Model capabilities lookup
The system SHALL provide an unexported `getModelCapabilities` function accepting a model ID string and returning metadata including `maxOutputTokens`, `supportsAdaptiveThinking`, `supportsStructuredOutput`, `rejectsSamplingParams`, `supportsXHighEffort`, `rejectsThinkingDisabledAboveHighEffort`, `rejectsThinkingDisabled`, `rejectsForcedToolUse`, `supportsBetweenToolsThinking` and `isKnownModel`. More-specific family checks SHALL precede generic Claude 4 base-family checks. Matching metadata SHALL NOT rewrite the request model ID.

The function SHALL classify the registered baseline families, plus `claude-sonnet-5-5` ported from `@ai-sdk/anthropic` 4.0.67 ahead of that baseline, as follows:
- IDs containing `claude-sonnet-5-5`: 128000 max output, adaptive and structured output, sampling rejected, xhigh effort supported, disabled thinking rejected above high effort, disabled and budget thinking rejected, forced tool use rejected, `between_tools` thinking supported, known. This row SHALL precede the `claude-sonnet-5` row.
- IDs containing `claude-opus-5`: 128000 max output, adaptive and structured output, sampling rejected, xhigh effort supported, disabled thinking rejected above high effort, known.
- IDs containing `claude-opus-4-8`, `claude-opus-4-7`, `claude-fable-5`, or `claude-sonnet-5`: 128000 max output, adaptive and structured output, sampling rejected, xhigh effort supported, known.
- IDs containing `claude-sonnet-4-6` or `claude-opus-4-6`: 128000 max output, adaptive and structured output, known; sampling not rejected.
- IDs containing `claude-sonnet-4-5`, `claude-opus-4-5`, or `claude-haiku-4-5`: 64000 max output, structured output, known; no adaptive thinking or sampling rejection.
- IDs containing `claude-opus-4-1`: 32000 max output, structured output, known; no adaptive thinking or sampling rejection.
- Other IDs matching `claude-sonnet-4` followed immediately by `-` or `@`: 64000 max output, known; no adaptive thinking, structured output or sampling rejection.
- Other IDs matching `claude-opus-4` followed immediately by `-` or `@`: 32000 max output, known; no adaptive thinking, structured output or sampling rejection.
- IDs containing `claude-3-haiku`: 4096 max output, known; no adaptive thinking, structured output or sampling rejection.
- Legacy Claude 2/3/instant families: 4096 max output and not known.
- Other IDs containing `claude-`: 128000 max output, adaptive and structured output, sampling rejected, xhigh effort supported, disabled thinking rejected above high effort, not known.
- All other IDs: 4096 max output and not known.

Boolean capabilities not explicitly enabled in a row SHALL be false. Bare `claude-sonnet-4` and `claude-opus-4` do not have the `-` or `@` boundary and SHALL remain in the unknown-Claude branch.

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
For a model with `rejectsThinkingDisabled`, the Anthropic provider SHALL rewrite an explicit provider-option thinking setting that the API would reject, and SHALL emit an `unsupported` warning with feature `providerOptions.anthropic.thinking` for each rewrite, as `@ai-sdk/anthropic` 4.0.67 does. Type `disabled` SHALL become `between_tools` when the model supports it, and SHALL otherwise be removed. Budget-based type `enabled` SHALL become `adaptive` without a budget. These rewrites SHALL run before the existing disabled-above-high effort cap and before the `between_tools` effort limit. The warning details SHALL name the model ID.

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
For a model with `rejectsForcedToolUse`, when no JSON response tool is used, the Anthropic provider SHALL send a `required` tool choice as `auto`, and a named tool choice as `auto` with only the named tool, as upstream `prepareTools` in `@ai-sdk/anthropic` 4.0.67 does. Each fallback SHALL emit an `unsupported` warning with feature `toolChoice` that tells the caller to instruct the model in the prompt and to verify the tool call. The named tool SHALL be matched after tool-name mapping, so a provider tool the caller renamed is kept. When the JSON response tool is used, it SHALL replace the caller's tool choice as the `anthropic-structured-output` capability specifies, and the caller's choice SHALL NOT filter tools or add its own warning.

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
