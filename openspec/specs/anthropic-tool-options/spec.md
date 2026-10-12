## Purpose

Define Anthropic tool-level provider options, eager input-streaming defaults, input-example passthrough, provider-defined tool behavior, and required beta headers.

## Requirements

### Requirement: Anthropic tool-level provider options extraction

`convertTools()` SHALL use `provider.ResolveOption[AnthropicToolOptions]` on function tools (empty/function type) at tool.ProviderOptions["anthropic"], accepting direct typed values and JSON-unmarshaled round-tripped RawProviderOption from prior SSE responses. Absent keys or malformed raw JSON SHALL mean empty options without error.

#### Scenario: Tool with deferLoading enabled

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` set to an `AnthropicToolOptions{DeferLoading: true}`
- **THEN** the resulting `BetaToolParam` SHALL have `DeferLoading` set to `true`

#### Scenario: Tool with allowedCallers

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` set to an `AnthropicToolOptions{AllowedCallers: ["direct", "code_execution_20250825"]}`
- **THEN** the resulting `BetaToolParam` SHALL have `AllowedCallers` set to `["direct", "code_execution_20250825"]`

#### Scenario: Tool with explicitly empty allowedCallers

- **WHEN** typed `AnthropicToolOptions{AllowedCallers: []string{}}` is serialized through `ProviderOptions` and resolved again
- **THEN** `AllowedCallers` SHALL remain a non-nil empty slice and the resulting native tool SHALL contain an empty `allowed_callers` array

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

DoStream with resolved ToolStreaming:true SHALL default eager_input_streaming:true on every function tool without explicit EagerInputStreaming, including synthetic JSON fallback for models without native output. DoGenerate or ToolStreaming:false SHALL NOT default it. Unset per-tool BetaToolParam.EagerInputStreaming SHALL follow that model-level default.

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

### Requirement: InputExamples passthrough

The Anthropic provider's `convertTools()` function SHALL pass `tool.InputExamples` through to `BetaToolParam.InputExamples` for function tools. Each `json.RawMessage` entry in `tool.InputExamples` SHALL be unmarshaled into `map[string]any` for the Anthropic SDK.

#### Scenario: Tool with input examples

- **WHEN** `convertTools()` receives a function tool with `InputExamples` containing `[{"x": 1}, {"x": 2}]`
- **THEN** the resulting `BetaToolParam` SHALL have `InputExamples` set to the corresponding `[]map[string]any`

#### Scenario: Tool with no input examples

- **WHEN** `convertTools()` receives a function tool with nil `InputExamples`
- **THEN** the resulting `BetaToolParam` SHALL have `InputExamples` unset

#### Scenario: Tool with explicitly empty input examples

- **WHEN** `convertTools()` receives a function tool with a non-nil, empty `InputExamples`
- **THEN** the resulting `BetaToolParam` SHALL contain an empty `InputExamples` array

#### Scenario: Tool with malformed input example entry

- **WHEN** `convertTools()` receives a function tool with an `InputExamples` entry that cannot be unmarshaled to `map[string]any`
- **THEN** that entry SHALL be skipped silently

### Requirement: Provider-defined tools unaffected

The `convertTools()` function SHALL NOT extract `AnthropicToolOptions` or `InputExamples` for provider-defined tools (tools with `Type == "provider-defined"`). Provider-defined tools SHALL continue to use their existing conversion paths unchanged.

#### Scenario: Provider-defined tool ignores provider options

- **WHEN** `convertTools()` receives a provider-defined tool with `ProviderOptions["anthropic"]` containing `{"deferLoading": true}`
- **THEN** the option SHALL be ignored and the tool SHALL be converted using its existing provider-defined path

### Requirement: Beta header auto-detection

convertTools SHALL return required betas with tools/warnings. Any function tool with explicitly empty InputExamples or a successfully converted example, or with non-nil AllowedCallers, SHALL select advanced-tool-use-2025-11-20. The caller SHALL merge/deduplicate these with AnthropicOptions.Betas in the anthropic-beta header.

#### Scenario: Beta auto-detection for inputExamples

- **WHEN** `convertTools()` receives a function tool with an explicitly empty `InputExamples` slice or at least one input example that converts successfully
- **THEN** the returned betas list SHALL include `"advanced-tool-use-2025-11-20"`

#### Scenario: All input examples are malformed

- **WHEN** `convertTools()` receives a non-empty `InputExamples` slice whose entries all fail conversion
- **THEN** those entries SHALL be omitted and the input-examples field SHALL NOT itself select `"advanced-tool-use-2025-11-20"`

#### Scenario: Beta auto-detection for allowedCallers

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` containing a present `allowedCallers` array, including an explicitly empty array
- **THEN** the returned betas list SHALL include `"advanced-tool-use-2025-11-20"`

#### Scenario: Beta deduplication

- **WHEN** `convertTools()` processes multiple tools where both `inputExamples` and `allowedCallers` are present
- **THEN** `"advanced-tool-use-2025-11-20"` SHALL appear only once in the returned betas

#### Scenario: No beta needed

- **WHEN** `convertTools()` processes tools with no `inputExamples` and no `allowedCallers`
- **THEN** the returned betas list SHALL be empty

### Requirement: Tool choice none keeps tools

When `ToolChoice` is `none` and the request has tools, `buildParams` SHALL keep the tools and set `tool_choice` to `{"type":"none"}` on the direct and Vertex transports, so tool definitions stay in the prompt cache prefix. `DisableParallelToolUse` SHALL NOT replace it. Without tools, `tool_choice` SHALL be omitted. Upstream `@ai-sdk/anthropic` removes the tools; `test/conformance/upstream.yaml` records the difference.

#### Scenario: None with tools

- **WHEN** a request has a function tool and `ToolChoice` is `none`
- **THEN** the tools SHALL be sent and `tool_choice` SHALL be `{"type":"none"}`

#### Scenario: None without tools

- **WHEN** a request has no tools and `ToolChoice` is `none`
- **THEN** `tool_choice` SHALL be omitted

### Requirement: Anthropic tool discovery and caller options

AnthropicToolOptions SHALL contain optional deferLoading bool (dynamic tool_search discovery), allowedCallers string array (which server tools can invoke it), and eagerInputStreaming bool (input streaming before completion).

#### Scenario: Anthropic tool discovery and caller options

- **WHEN** a function tool configures deferLoading:true, allowedCallers:["direct"] and eagerInputStreaming:true
- **THEN** BetaToolParam SHALL contain DeferLoading:true, AllowedCallers:["direct"] and EagerInputStreaming:true

### Requirement: Anthropic allowed-callers presence preservation

AnthropicToolOptions.AllowedCallers nil SHALL be omitted; explicit empty non-nil slices SHALL serialize as "allowedCallers":[] and remain distinct through ProviderOptions JSON round trip and ResolveOption[AnthropicToolOptions].

#### Scenario: Anthropic allowed-callers presence preservation

- **WHEN** typed options have an empty non-nil AllowedCallers slice
- **THEN** JSON round trip SHALL retain the empty slice and native allowed_callers:[] rather than omit it

### Requirement: Anthropic explicit eager-input truthiness

Explicit EagerInputStreaming:true SHALL emit eager_input_streaming:true even with model default false. Explicit false SHALL override model default true and omit the wire field, never emit eager_input_streaming:false. Only truthy resolved values SHALL emit true, matching upstream `...(eagerInputStreaming ? { eager_input_streaming: true } : {})`.

#### Scenario: Anthropic explicit eager-input truthiness

- **WHEN** per-tool eagerInputStreaming:false is set on a default-enabled stream
- **THEN** the native field SHALL be omitted, while explicit true with a disabled model default SHALL emit true
