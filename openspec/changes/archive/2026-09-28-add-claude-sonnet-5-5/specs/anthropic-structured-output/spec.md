## MODIFIED Requirements

### Requirement: Tool choice override in fallback mode

When the tool-based fallback is active, `buildParams` SHALL override `ToolChoice` to `required` (OfAny) with `DisableParallelToolUse` set to `true`, regardless of the caller's original `ToolChoice` setting. On a model that rejects forced tool use, the override SHALL instead be `auto` (OfAuto) with `DisableParallelToolUse` set to `true`, with one unsupported `toolChoice` warning for `required`, as `@ai-sdk/anthropic` 4.0.67 does. The caller's original tool choice SHALL NOT filter tools or add a warning of its own.

#### Scenario: Tool choice forced to required

- **WHEN** the tool-based fallback is active and the caller set `ToolChoice` to `auto`
- **THEN** `ToolChoice` SHALL be overridden to `OfAny` with `DisableParallelToolUse: true`

#### Scenario: Tool choice override when caller set none

- **WHEN** the tool-based fallback is active and the caller set `ToolChoice` to `none`
- **THEN** `ToolChoice` SHALL be overridden to `OfAny` with `DisableParallelToolUse: true`

#### Scenario: Tool choice auto on a model that rejects forced tool use
- **WHEN** the tool-based fallback is active for `claude-sonnet-5-5` on a provider transport without native structured output
- **THEN** `ToolChoice` SHALL be `OfAuto` with `DisableParallelToolUse: true`
- **AND** one unsupported warning with feature `toolChoice` SHALL be emitted

#### Scenario: Caller forced choice with the fallback on Sonnet 5.5
- **WHEN** the tool-based fallback is active for `claude-sonnet-5-5` and the caller set `ToolChoice` to `tool` for one of several tools
- **THEN** all caller tools and the `json` tool SHALL be sent
- **AND** exactly one `toolChoice` warning, for `required`, SHALL be emitted

## ADDED Requirements

### Requirement: JSON tool mode on models that reject forced tool use
When `structuredOutputMode` is `jsonTool` and the model rejects forced tool use while the model and provider transport both support native structured output, `buildParams` SHALL use native JSON schema mode instead, as `@ai-sdk/anthropic` 4.0.67 does. It SHALL emit an `unsupported` warning with feature `providerOptions.anthropic.structuredOutputMode` and details `structuredOutputMode 'jsonTool' is not supported by <modelID> because it rejects forced tool use. Using 'outputFormat' instead.`

#### Scenario: jsonTool mode on direct Sonnet 5.5
- **WHEN** the direct Anthropic provider calls `claude-sonnet-5-5` with a JSON schema response and `structuredOutputMode: jsonTool`
- **THEN** `OutputConfig.Format` SHALL contain the sanitized schema and no `json` tool SHALL be added
- **AND** the warning SHALL name `claude-sonnet-5-5`

#### Scenario: jsonTool mode on Vertex Sonnet 5.5
- **WHEN** the Vertex provider calls `claude-sonnet-5-5` with a JSON schema response and `structuredOutputMode: jsonTool`
- **THEN** `OutputConfig.Format` SHALL contain the sanitized schema, no `json` tool SHALL be added, and the same warning SHALL be emitted

#### Scenario: jsonTool mode without native transport support
- **WHEN** a provider transport without native structured output calls `claude-sonnet-5-5` with a JSON schema response
- **THEN** the tool-based fallback SHALL be used with an `auto` tool choice
