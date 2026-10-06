## ADDED Requirements

### Requirement: Anthropic tool-level provider options extraction

The Anthropic provider's `convertTools()` function SHALL extract Anthropic-specific options from `tool.ProviderOptions["anthropic"]` for function tools (tools with empty or `"function"` type). The options SHALL be deserialized into an `AnthropicToolOptions` struct with the following fields:
- `deferLoading` (bool, optional) -- controls whether the tool is deferred for dynamic discovery via `tool_search`
- `allowedCallers` (string array, optional) -- restricts which server tools can invoke this tool
- `eagerInputStreaming` (bool, optional) -- enables streaming tool input before completion

If the `"anthropic"` key is absent or the JSON is malformed, the options SHALL be treated as empty (no error produced).

#### Scenario: Tool with deferLoading enabled

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` containing `{"deferLoading": true}`
- **THEN** the resulting `BetaToolParam` SHALL have `DeferLoading` set to `true`

#### Scenario: Tool with allowedCallers

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` containing `{"allowedCallers": ["direct", "code_execution_20250825"]}`
- **THEN** the resulting `BetaToolParam` SHALL have `AllowedCallers` set to `["direct", "code_execution_20250825"]`

#### Scenario: Tool with eagerInputStreaming enabled

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` containing `{"eagerInputStreaming": true}`
- **THEN** the resulting `BetaToolParam` SHALL have `EagerInputStreaming` set to `true`

#### Scenario: Tool with all three options

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` containing `{"deferLoading": true, "allowedCallers": ["direct"], "eagerInputStreaming": true}`
- **THEN** the resulting `BetaToolParam` SHALL have all three fields set accordingly

#### Scenario: Tool with no Anthropic provider options

- **WHEN** `convertTools()` receives a function tool with no `"anthropic"` key in `ProviderOptions`
- **THEN** the resulting `BetaToolParam` SHALL have `DeferLoading`, `AllowedCallers`, and `EagerInputStreaming` unset (zero values)

#### Scenario: Tool with malformed Anthropic provider options

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` containing invalid JSON
- **THEN** the options SHALL be treated as empty and no error SHALL be produced

### Requirement: InputExamples passthrough

The Anthropic provider's `convertTools()` function SHALL pass `tool.InputExamples` through to `BetaToolParam.InputExamples` for function tools. Each `json.RawMessage` entry in `tool.InputExamples` SHALL be unmarshaled into `map[string]any` for the Anthropic SDK.

#### Scenario: Tool with input examples

- **WHEN** `convertTools()` receives a function tool with `InputExamples` containing `[{"x": 1}, {"x": 2}]`
- **THEN** the resulting `BetaToolParam` SHALL have `InputExamples` set to the corresponding `[]map[string]any`

#### Scenario: Tool with no input examples

- **WHEN** `convertTools()` receives a function tool with nil or empty `InputExamples`
- **THEN** the resulting `BetaToolParam` SHALL have `InputExamples` unset

#### Scenario: Tool with malformed input example entry

- **WHEN** `convertTools()` receives a function tool with an `InputExamples` entry that cannot be unmarshaled to `map[string]any`
- **THEN** that entry SHALL be skipped silently

### Requirement: Provider-defined tools unaffected

The `convertTools()` function SHALL NOT extract `AnthropicToolOptions` or `InputExamples` for provider-defined tools (tools with `Type == "provider-defined"`). Provider-defined tools SHALL continue to use their existing conversion paths unchanged.

#### Scenario: Provider-defined tool ignores provider options

- **WHEN** `convertTools()` receives a provider-defined tool with `ProviderOptions["anthropic"]` containing `{"deferLoading": true}`
- **THEN** the option SHALL be ignored and the tool SHALL be converted using its existing provider-defined path

### Requirement: Beta header auto-detection

The `convertTools()` function SHALL return a list of required beta header strings alongside the converted tools and warnings. The following auto-detection rules SHALL apply:
- When any function tool has non-nil `InputExamples`, add `"advanced-tool-use-2025-11-20"`
- When any function tool has non-empty `AllowedCallers` (from `AnthropicToolOptions`), add `"advanced-tool-use-2025-11-20"`

The caller SHALL merge auto-detected betas with any explicit betas from `AnthropicOptions.Betas` and apply them as the `anthropic-beta` request header, deduplicating entries.

#### Scenario: Beta auto-detection for inputExamples

- **WHEN** `convertTools()` receives a function tool with non-nil `InputExamples`
- **THEN** the returned betas list SHALL include `"advanced-tool-use-2025-11-20"`

#### Scenario: Beta auto-detection for allowedCallers

- **WHEN** `convertTools()` receives a function tool with `ProviderOptions["anthropic"]` containing `{"allowedCallers": ["direct"]}`
- **THEN** the returned betas list SHALL include `"advanced-tool-use-2025-11-20"`

#### Scenario: Beta deduplication

- **WHEN** `convertTools()` processes multiple tools where both `inputExamples` and `allowedCallers` are present
- **THEN** `"advanced-tool-use-2025-11-20"` SHALL appear only once in the returned betas

#### Scenario: No beta needed

- **WHEN** `convertTools()` processes tools with no `inputExamples` and no `allowedCallers`
- **THEN** the returned betas list SHALL be empty
