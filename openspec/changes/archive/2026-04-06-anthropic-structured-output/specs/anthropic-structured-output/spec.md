## ADDED Requirements

### Requirement: Native JSON schema mode

When `CallOptions.ResponseFormat` has `Type: "json"` and a non-nil `Schema`, and the model supports native structured output, `buildParams` SHALL set `OutputConfig.Format` using the SDK's `BetaJSONSchemaOutputFormat()` helper with the schema from `ResponseFormat.Schema`. No tool injection or tool choice modification SHALL occur.

#### Scenario: Native mode on a supported model

- **WHEN** `buildParams` is called with `ResponseFormat` having `Type: "json"` and a valid JSON schema, and the model ID is `claude-sonnet-4-5`
- **THEN** the returned `BetaMessageNewParams.OutputConfig.Format` SHALL be set to a `BetaJSONOutputFormatParam` with the schema
- **AND** no synthetic tool SHALL be appended to the tools list
- **AND** `ToolChoice` SHALL remain unchanged from the caller's setting

#### Scenario: Native mode preserves existing OutputConfig.Effort

- **WHEN** `buildParams` is called with both `ResponseFormat` (json + schema) and provider options that set `effort`
- **THEN** `OutputConfig` SHALL include both `Effort` and `Format` fields

### Requirement: Tool-based JSON fallback

When `CallOptions.ResponseFormat` has `Type: "json"` and a non-nil `Schema`, and the model does not support native structured output, `buildParams` SHALL synthesize a tool named `"json"` with `description: "Respond with a JSON object."` and the `ResponseFormat.Schema` as `inputSchema`. The tool SHALL be appended to the existing tools list (after any user-defined tools).

#### Scenario: Tool injection on an older model

- **WHEN** `buildParams` is called with `ResponseFormat` having `Type: "json"` and a valid JSON schema, and the model ID is `claude-3-haiku`
- **THEN** the returned `BetaMessageNewParams.Tools` SHALL include the user's tools plus a synthetic tool with name `"json"`, description `"Respond with a JSON object."`, and `InputSchema` matching the provided schema
- **AND** `OutputConfig.Format` SHALL NOT be set

#### Scenario: Tool injection with no existing user tools

- **WHEN** `buildParams` is called with `ResponseFormat` (json + schema), no user tools, and a model that does not support native structured output
- **THEN** the tools list SHALL contain exactly one tool: the synthetic `"json"` tool

### Requirement: Tool choice override in fallback mode

When the tool-based fallback is active, `buildParams` SHALL override `ToolChoice` to `required` (OfAny) with `DisableParallelToolUse` set to `true`, regardless of the caller's original `ToolChoice` setting.

#### Scenario: Tool choice forced to required

- **WHEN** the tool-based fallback is active and the caller set `ToolChoice` to `auto`
- **THEN** `ToolChoice` SHALL be overridden to `OfAny` with `DisableParallelToolUse: true`

#### Scenario: Tool choice override when caller set none

- **WHEN** the tool-based fallback is active and the caller set `ToolChoice` to `none`
- **THEN** `ToolChoice` SHALL be overridden to `OfAny` with `DisableParallelToolUse: true`

### Requirement: Stream remapping for JSON response tool

When the json response tool was injected (tool-based fallback), the `streamAdapter` SHALL intercept content blocks for the `"json"` tool and remap them to text content:
- `tool_use` content block start for `"json"` SHALL be suppressed (no `PartToolInputStart` emitted)
- `input_json_delta` deltas for the json tool SHALL be emitted as `PartTextDelta` instead of `PartToolInputDelta`
- `content_block_stop` for the json tool SHALL be suppressed (no `PartToolInputEnd` or `PartToolCall` emitted)

#### Scenario: Tool input streamed as text

- **WHEN** the json response tool is active and the model streams `input_json_delta` events for the `"json"` tool
- **THEN** the stream adapter SHALL emit `PartTextDelta` parts with the delta text
- **AND** SHALL NOT emit `PartToolInputStart`, `PartToolInputDelta`, `PartToolInputEnd`, or `PartToolCall` for the json tool

#### Scenario: Non-json tools unaffected

- **WHEN** the json response tool is active and the model calls a user-defined tool
- **THEN** the stream adapter SHALL emit `PartToolInputStart`, `PartToolInputDelta`, `PartToolInputEnd`, and `PartToolCall` normally for that tool

### Requirement: Finish reason remapping for JSON response tool

When the json response tool was injected and the model's stop reason is `tool_use`, the stream adapter SHALL remap the finish reason to `stop` so the orchestration layer treats it as a completed text response.

#### Scenario: Stop reason remapped from tool_use to stop

- **WHEN** the json response tool is active and the model finishes with `stop_reason: tool_use`
- **THEN** the stream adapter SHALL emit `PartFinish` with `FinishReason: "stop"`

#### Scenario: Stop reason preserved when not tool_use

- **WHEN** the json response tool is active and the model finishes with `stop_reason: end_turn`
- **THEN** the stream adapter SHALL emit `PartFinish` with the original finish reason

### Requirement: JSON response tool state passing

`buildParams` SHALL signal to the caller whether a json response tool was injected. The `streamAdapter` SHALL receive this signal at construction and use it to gate stream remapping logic.

#### Scenario: State passed when tool injected

- **WHEN** `buildParams` injects the json response tool
- **THEN** the returned state SHALL indicate `usesJsonResponseTool: true`
- **AND** the `streamAdapter` SHALL be constructed with this flag

#### Scenario: State not set when native mode used

- **WHEN** `buildParams` uses native `OutputConfig.Format`
- **THEN** the returned state SHALL indicate `usesJsonResponseTool: false`

### Requirement: Schemaless JSON mode unsupported

When `CallOptions.ResponseFormat` has `Type: "json"` but `Schema` is nil, `buildParams` SHALL emit a warning with type `unsupported` (matching `provider.WarnUnsupported`), feature `responseFormat`, and a message indicating that schemaless JSON mode is not supported by Anthropic. No structured output handling SHALL occur.

#### Scenario: Schemaless JSON emits warning

- **WHEN** `buildParams` is called with `ResponseFormat` having `Type: "json"` and nil `Schema`
- **THEN** a warning SHALL be emitted with feature `responseFormat`
- **AND** no `OutputConfig.Format` or synthetic tool SHALL be set

### Requirement: Text response format is a no-op

When `CallOptions.ResponseFormat` has `Type: "text"`, `buildParams` SHALL not modify the request. No warning SHALL be emitted.

#### Scenario: Text format ignored silently

- **WHEN** `buildParams` is called with `ResponseFormat` having `Type: "text"`
- **THEN** no warning SHALL be emitted
- **AND** no `OutputConfig.Format` or synthetic tool SHALL be set
