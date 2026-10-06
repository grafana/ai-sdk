## ADDED Requirements

### Requirement: Model-level `ToolStreaming` option

The Anthropic provider's `AnthropicOptions` (read from `CallOptions.ProviderOptions["anthropic"]`) SHALL accept a `ToolStreaming *bool` field (JSON key `toolStreaming`) that controls whether function tools receive a default `eager_input_streaming: true` on streaming requests. A `nil` value SHALL be treated as `true` (matching upstream's `?? true` semantics).

#### Scenario: ToolStreaming unset defaults to enabled

- **WHEN** `AnthropicOptions.ToolStreaming` is `nil`
- **THEN** the resolved tool-streaming flag SHALL be `true`

#### Scenario: ToolStreaming explicitly true

- **WHEN** `AnthropicOptions.ToolStreaming` points to `true`
- **THEN** the resolved tool-streaming flag SHALL be `true`

#### Scenario: ToolStreaming explicitly false

- **WHEN** `AnthropicOptions.ToolStreaming` points to `false`
- **THEN** the resolved tool-streaming flag SHALL be `false`

### Requirement: Default `eager_input_streaming` on streaming requests

When the provider is invoked via `DoStream` and the resolved `ToolStreaming` flag is `true`, the Anthropic provider SHALL emit `eager_input_streaming: true` on every function tool in the request that does NOT explicitly set `AnthropicToolOptions.EagerInputStreaming`, including the synthetic JSON fallback tool used for structured-output models that lack native structured-output support. When the request is invoked via `DoGenerate`, or when the resolved `ToolStreaming` flag is `false`, the provider SHALL NOT default `eager_input_streaming` on function tools. Per-tool explicit `EagerInputStreaming` values resolve as follows, matching upstream `...(eagerInputStreaming ? { eager_input_streaming: true } : {})`:
- An explicit `true` SHALL emit `eager_input_streaming: true` (overriding a model-level `false`).
- An explicit `false` SHALL OMIT the `eager_input_streaming` field entirely (overriding a model-level `true`); the field SHALL NOT be sent as `eager_input_streaming: false`.

#### Scenario: Streaming with default ToolStreaming and tool without explicit eagerInputStreaming

- **WHEN** `DoStream` is called with `AnthropicOptions.ToolStreaming = nil` and a function tool whose `ProviderOptions["anthropic"]` does not set `eagerInputStreaming`
- **THEN** the resulting `BetaToolParam` SHALL have `EagerInputStreaming` set to `true`

#### Scenario: Streaming with ToolStreaming disabled

- **WHEN** `DoStream` is called with `AnthropicOptions.ToolStreaming` pointing to `false` and a function tool whose `ProviderOptions["anthropic"]` does not set `eagerInputStreaming`
- **THEN** the resulting `BetaToolParam` SHALL NOT have `EagerInputStreaming` set

#### Scenario: Generate (non-streaming) never defaults eager streaming

- **WHEN** `DoGenerate` is called with any value of `AnthropicOptions.ToolStreaming` and a function tool whose `ProviderOptions["anthropic"]` does not set `eagerInputStreaming`
- **THEN** the resulting `BetaToolParam` SHALL NOT have `EagerInputStreaming` set

#### Scenario: Per-tool explicit false suppresses the model-level default

- **WHEN** `DoStream` is called with `AnthropicOptions.ToolStreaming = nil` and a function tool whose `ProviderOptions["anthropic"]` sets `eagerInputStreaming: false`
- **THEN** the resulting `BetaToolParam` SHALL NOT have `EagerInputStreaming` set (the field SHALL be omitted from the wire payload, not sent as `false`)

#### Scenario: Per-tool explicit true wins when ToolStreaming is disabled

- **WHEN** `DoStream` is called with `AnthropicOptions.ToolStreaming` pointing to `false` and a function tool whose `ProviderOptions["anthropic"]` sets `eagerInputStreaming: true`
- **THEN** the resulting `BetaToolParam` SHALL have `EagerInputStreaming` set to `true`

#### Scenario: JSON response-format fallback tool receives the default on streaming

- **WHEN** `DoStream` is called with a `ResponseFormat.Type = "json"` and a non-native-structured-output model (e.g., `claude-3-haiku`), triggering the synthetic `"json"` fallback tool
- **THEN** the appended `"json"` `BetaToolParam` SHALL have `EagerInputStreaming` set to `true`

#### Scenario: JSON response-format fallback tool respects ToolStreaming=false

- **WHEN** `DoStream` is called with a `ResponseFormat.Type = "json"` on a non-native-structured-output model and `AnthropicOptions.ToolStreaming` points to `false`
- **THEN** the appended `"json"` `BetaToolParam` SHALL NOT have `EagerInputStreaming` set

#### Scenario: JSON response-format fallback tool not defaulted on DoGenerate

- **WHEN** `DoGenerate` is called with a `ResponseFormat.Type = "json"` on a non-native-structured-output model
- **THEN** the appended `"json"` `BetaToolParam` SHALL NOT have `EagerInputStreaming` set

#### Scenario: Provider-defined tools are not affected

- **WHEN** `DoStream` is called with `AnthropicOptions.ToolStreaming = nil` and a provider-defined tool (e.g., `anthropic.web_search_20250305`)
- **THEN** the converted provider-defined tool SHALL NOT have `EagerInputStreaming` set, regardless of the model-level default

## MODIFIED Requirements

### Requirement: Anthropic tool-level provider options extraction

The Anthropic provider's `convertTools()` function SHALL extract Anthropic-specific options from `tool.ProviderOptions["anthropic"]` for function tools (tools with empty or `"function"` type). The provider SHALL use `provider.ResolveOption[AnthropicToolOptions]` to handle both typed options (direct `AnthropicToolOptions` values) and round-tripped raw options (`RawProviderOption` from previous SSE responses, unmarshaled via JSON). The `AnthropicToolOptions` struct contains fields:
- `deferLoading` (bool, optional) -- controls whether the tool is deferred for dynamic discovery via `tool_search`
- `allowedCallers` (string array, optional) -- restricts which server tools can invoke this tool
- `eagerInputStreaming` (bool, optional) -- enables streaming tool input before completion. When unset, the resulting `BetaToolParam.EagerInputStreaming` is determined by the model-level default (see "Default `eager_input_streaming` on streaming requests"). When set to `false`, the field SHALL be omitted from the wire payload (matching upstream `...(eagerInputStreaming ? { eager_input_streaming: true } : {})`); only a truthy resolved value emits `eager_input_streaming: true`.

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

#### Scenario: Tool with no Anthropic provider options in non-streaming context

- **WHEN** `convertTools()` is invoked from `DoGenerate` with a function tool that has no `"anthropic"` key in `ProviderOptions`
- **THEN** the resulting `BetaToolParam` SHALL have `DeferLoading`, `AllowedCallers`, and `EagerInputStreaming` unset (zero values)

#### Scenario: Tool with no Anthropic provider options in streaming context

- **WHEN** `convertTools()` is invoked from `DoStream` (with the default `ToolStreaming` of `nil`) and receives a function tool that has no `"anthropic"` key in `ProviderOptions`
- **THEN** the resulting `BetaToolParam` SHALL have `DeferLoading` and `AllowedCallers` unset, and `EagerInputStreaming` SHALL be set to `true` (from the model-level default)

#### Scenario: Tool with malformed raw provider options

- **WHEN** `convertTools()` receives a function tool with a `RawProviderOption` at key `"anthropic"` containing invalid JSON
- **THEN** the options SHALL be treated as empty and no error SHALL be produced (model-level defaults still apply normally for `EagerInputStreaming`)
