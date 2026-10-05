## MODIFIED Requirements

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

## ADDED Requirements

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
