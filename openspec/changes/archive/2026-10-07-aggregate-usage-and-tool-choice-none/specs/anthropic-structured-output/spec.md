## MODIFIED Requirements

### Requirement: Tool choice override in fallback mode

When the tool-based fallback is active, `buildParams` SHALL override `ToolChoice` to `required` (OfAny) with `DisableParallelToolUse` set to `true`, regardless of the caller's original `ToolChoice` setting. On a model that rejects forced tool use, the override SHALL instead be `auto` (OfAuto) with `DisableParallelToolUse` set to `true`, with one unsupported `toolChoice` warning for `required`, as `@ai-sdk/anthropic` 4.0.67 does. Apart from `none`, which sends only the `json` tool, the caller's tool choice SHALL NOT filter tools or add a warning of its own.

#### Scenario: Tool choice forced to required

- **WHEN** the tool-based fallback is active and the caller set `ToolChoice` to `auto`
- **THEN** `ToolChoice` SHALL be overridden to `OfAny` with `DisableParallelToolUse: true`

#### Scenario: Tool choice override when caller set none

- **WHEN** the tool-based fallback is active and the caller set `ToolChoice` to `none` with caller tools
- **THEN** `ToolChoice` SHALL be overridden to `OfAny` with `DisableParallelToolUse: true`
- **AND** the tools list SHALL contain only the `json` tool

#### Scenario: Tool choice auto on a model that rejects forced tool use
- **WHEN** the tool-based fallback is active for `claude-sonnet-5-5` on a provider transport without native structured output
- **THEN** `ToolChoice` SHALL be `OfAuto` with `DisableParallelToolUse: true`
- **AND** one unsupported warning with feature `toolChoice` SHALL be emitted

#### Scenario: Caller forced choice with the fallback on Sonnet 5.5
- **WHEN** the tool-based fallback is active for `claude-sonnet-5-5` and the caller set `ToolChoice` to `tool` for one of several tools
- **THEN** all caller tools and the `json` tool SHALL be sent
- **AND** exactly one `toolChoice` warning, for `required`, SHALL be emitted
