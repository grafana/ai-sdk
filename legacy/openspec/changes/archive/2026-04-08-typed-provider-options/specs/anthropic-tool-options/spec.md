## MODIFIED Requirements

### Requirement: Anthropic tool-level provider options extraction

The Anthropic provider's `convertTools()` function SHALL extract Anthropic-specific options from `tool.ProviderOptions["anthropic"]` for function tools (tools with empty or `"function"` type). The provider SHALL use `provider.ResolveOption[AnthropicToolOptions]` to handle both typed options (direct `AnthropicToolOptions` values) and round-tripped raw options (`RawProviderOption` from previous SSE responses, unmarshaled via JSON). The `AnthropicToolOptions` struct contains fields:
- `deferLoading` (bool, optional) -- controls whether the tool is deferred for dynamic discovery via `tool_search`
- `allowedCallers` (string array, optional) -- restricts which server tools can invoke this tool
- `eagerInputStreaming` (bool, optional) -- enables streaming tool input before completion

If the `"anthropic"` key is absent, the options SHALL be treated as empty. If the value is a `RawProviderOption` with malformed JSON, the options SHALL be treated as empty (no error produced).

#### Scenario: Tool with deferLoading enabled

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` set to an `AnthropicToolOptions{DeferLoading: true}`
- **THEN** the resulting `BetaToolParam` SHALL have `DeferLoading` set to `true`

#### Scenario: Tool with allowedCallers

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` set to an `AnthropicToolOptions{AllowedCallers: ["direct", "code_execution_20250825"]}`
- **THEN** the resulting `BetaToolParam` SHALL have `AllowedCallers` set to `["direct", "code_execution_20250825"]`

#### Scenario: Tool with eagerInputStreaming enabled

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` set to an `AnthropicToolOptions{EagerInputStreaming: true}`
- **THEN** the resulting `BetaToolParam` SHALL have `EagerInputStreaming` set to `true`

#### Scenario: Tool with all three options

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` set to an `AnthropicToolOptions` with all three fields set
- **THEN** the resulting `BetaToolParam` SHALL have all three fields set accordingly

#### Scenario: Tool with no Anthropic provider options

- **WHEN** `convertTools()` receives a function tool with no `"anthropic"` key in `ProviderOptions`
- **THEN** the resulting `BetaToolParam` SHALL have `DeferLoading`, `AllowedCallers`, and `EagerInputStreaming` unset (zero values)

#### Scenario: Tool with malformed raw provider options

- **WHEN** `convertTools()` receives a function tool with a `RawProviderOption` at key `"anthropic"` containing invalid JSON
- **THEN** the options SHALL be treated as empty and no error SHALL be produced
