## ADDED Requirements

### Requirement: Code execution tool definitions

The Anthropic provider SHALL support the following code execution provider-defined tool IDs in `convertProviderTool`:

| Tool ID | API type | API name | Beta header |
|---|---|---|---|
| `anthropic.code_execution_20250522` | `code_execution_20250522` | `code_execution` | `code-execution-2025-05-22` |
| `anthropic.code_execution_20250825` | `code_execution_20250825` | `code_execution` | `code-execution-2025-08-25` |
| `anthropic.code_execution_20260120` | `code_execution_20260120` | `code_execution` | (none) |

Each SHALL produce the corresponding Anthropic SDK tool type with `name` set to `"code_execution"` and return the beta header string (if any) for inclusion in the API request.

#### Scenario: code_execution_20250522 tool definition

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.code_execution_20250522"`
- **THEN** it produces a tool with `type: "code_execution_20250522"` and `name: "code_execution"`
- **AND** returns beta `"code-execution-2025-05-22"`

#### Scenario: code_execution_20250825 tool definition

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.code_execution_20250825"`
- **THEN** it produces a tool with `type: "code_execution_20250825"` and `name: "code_execution"`
- **AND** returns beta `"code-execution-2025-08-25"`

#### Scenario: code_execution_20260120 tool definition

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.code_execution_20260120"`
- **THEN** it produces a tool with `type: "code_execution_20260120"` and `name: "code_execution"`
- **AND** returns no beta header

### Requirement: Computer use tool definitions

The Anthropic provider SHALL support the following computer use provider-defined tool IDs in `convertProviderTool`:

| Tool ID | API type | API name | Beta header | Args |
|---|---|---|---|---|
| `anthropic.computer_20241022` | `computer_20241022` | `computer` | `computer-use-2024-10-22` | `displayWidthPx`, `displayHeightPx`, `displayNumber` |
| `anthropic.computer_20250124` | `computer_20250124` | `computer` | `computer-use-2025-01-24` | `displayWidthPx`, `displayHeightPx`, `displayNumber` |
| `anthropic.computer_20251124` | `computer_20251124` | `computer` | `computer-use-2025-11-24` | `displayWidthPx`, `displayHeightPx`, `displayNumber`, `enableZoom` |

Dimension args (`displayWidthPx`, `displayHeightPx`) SHALL be extracted from the tool's `Args` map as numeric values. `displayNumber` SHALL be extracted as a numeric value when present. The `20251124` version SHALL additionally extract `enableZoom` as a boolean and map it to `enable_zoom`.

#### Scenario: computer_20241022 with display dimensions

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.computer_20241022"` and `Args: {"displayWidthPx": 1920, "displayHeightPx": 1080}`
- **THEN** it produces a tool with `type: "computer_20241022"`, `name: "computer"`, `display_width_px: 1920`, `display_height_px: 1080`
- **AND** returns beta `"computer-use-2024-10-22"`

#### Scenario: computer_20250124 with display number

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.computer_20250124"` and `Args: {"displayWidthPx": 1920, "displayHeightPx": 1080, "displayNumber": 1}`
- **THEN** it produces a tool with `type: "computer_20250124"`, `name: "computer"`, `display_width_px: 1920`, `display_height_px: 1080`, `display_number: 1`
- **AND** returns beta `"computer-use-2025-01-24"`

#### Scenario: computer_20251124 with enable zoom

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.computer_20251124"` and `Args: {"displayWidthPx": 1920, "displayHeightPx": 1080, "enableZoom": true}`
- **THEN** it produces a tool with `type: "computer_20251124"`, `name: "computer"`, `display_width_px: 1920`, `display_height_px: 1080`, `enable_zoom: true`
- **AND** returns beta `"computer-use-2025-11-24"`

### Requirement: Text editor tool definitions

The Anthropic provider SHALL support the following text editor provider-defined tool IDs in `convertProviderTool`:

| Tool ID | API type | API name | Beta header | Args |
|---|---|---|---|---|
| `anthropic.text_editor_20241022` | `text_editor_20241022` | `str_replace_editor` | `computer-use-2024-10-22` | (none) |
| `anthropic.text_editor_20250124` | `text_editor_20250124` | `str_replace_editor` | `computer-use-2025-01-24` | (none) |
| `anthropic.text_editor_20250429` | `text_editor_20250429` | `str_replace_based_edit_tool` | `computer-use-2025-01-24` | (none) |
| `anthropic.text_editor_20250728` | `text_editor_20250728` | `str_replace_based_edit_tool` | (none) | `maxCharacters` |

Note the API name difference: `20241022` and `20250124` use `str_replace_editor`, while `20250429` and `20250728` use `str_replace_based_edit_tool`. The `20250728` version SHALL extract `maxCharacters` from `Args` as a numeric value and map it to `max_characters`.

#### Scenario: text_editor_20241022 definition

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.text_editor_20241022"`
- **THEN** it produces a tool with `type: "text_editor_20241022"` and `name: "str_replace_editor"`
- **AND** returns beta `"computer-use-2024-10-22"`

#### Scenario: text_editor_20250429 definition

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.text_editor_20250429"`
- **THEN** it produces a tool with `type: "text_editor_20250429"` and `name: "str_replace_based_edit_tool"`
- **AND** returns beta `"computer-use-2025-01-24"`

#### Scenario: text_editor_20250728 with maxCharacters

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.text_editor_20250728"` and `Args: {"maxCharacters": 50000}`
- **THEN** it produces a tool with `type: "text_editor_20250728"`, `name: "str_replace_based_edit_tool"`, `max_characters: 50000`
- **AND** returns no beta header

### Requirement: Bash tool definitions

The Anthropic provider SHALL support the following bash provider-defined tool IDs in `convertProviderTool`:

| Tool ID | API type | API name | Beta header |
|---|---|---|---|
| `anthropic.bash_20241022` | `bash_20241022` | `bash` | `computer-use-2024-10-22` |
| `anthropic.bash_20250124` | `bash_20250124` | `bash` | `computer-use-2025-01-24` |

#### Scenario: bash_20241022 definition

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.bash_20241022"`
- **THEN** it produces a tool with `type: "bash_20241022"` and `name: "bash"`
- **AND** returns beta `"computer-use-2024-10-22"`

#### Scenario: bash_20250124 definition

- **WHEN** `convertProviderTool` receives a provider tool with `ID: "anthropic.bash_20250124"`
- **THEN** it produces a tool with `type: "bash_20250124"` and `name: "bash"`
- **AND** returns beta `"computer-use-2025-01-24"`

### Requirement: Code execution delta rewriting in streaming

When streaming responses for `server_tool_use` blocks with wire name `bash_code_execution` or `text_editor_code_execution`, the stream adapter SHALL rewrite the first non-empty `input_json_delta` to inject a `type` field. The rewriting SHALL replace the opening `{` of the first non-empty delta with `{"type": "<providerToolName>",` where `<providerToolName>` is the original wire name (`bash_code_execution` or `text_editor_code_execution`).

The stream adapter SHALL track `firstDelta bool` and `providerToolName string` per content block in `blockState`. Empty deltas (zero length) SHALL be skipped for the purpose of identifying the "first" delta. After the first non-empty delta is rewritten, `firstDelta` SHALL be set to `false`.

The tool name emitted to the orchestration layer SHALL map through `code_execution` (not the original wire name), matching the upstream behavior where `bash_code_execution` and `text_editor_code_execution` are sub-types of `code_execution`.

#### Scenario: First non-empty delta rewritten for bash_code_execution

- **WHEN** a `server_tool_use` block starts with wire name `bash_code_execution`
- **AND** the first non-empty `input_json_delta` arrives with value `{"code": "ls -la"}`
- **THEN** the delta is rewritten to `{"type": "bash_code_execution","code": "ls -la"}`
- **AND** the emitted `PartToolInputStart` and `PartToolCall` use `mapping.toCustomToolName("code_execution")` as the tool name

#### Scenario: First non-empty delta rewritten for text_editor_code_execution

- **WHEN** a `server_tool_use` block starts with wire name `text_editor_code_execution`
- **AND** the first non-empty `input_json_delta` arrives with value `{"command": "view", "path": "/tmp"}`
- **THEN** the delta is rewritten to `{"type": "text_editor_code_execution","command": "view", "path": "/tmp"}`

#### Scenario: Empty deltas skipped before first rewrite

- **WHEN** a `server_tool_use` block starts with wire name `bash_code_execution`
- **AND** the first `input_json_delta` has an empty string value
- **AND** the second `input_json_delta` has value `{"code": "echo hi"}`
- **THEN** only the second delta is rewritten with the type injection
- **AND** the empty delta is not emitted as `PartToolInputDelta`

#### Scenario: Subsequent deltas not rewritten

- **WHEN** a `server_tool_use` block with wire name `bash_code_execution` has already received its first non-empty delta
- **AND** another `input_json_delta` arrives
- **THEN** it is emitted as-is without rewriting

### Requirement: Programmatic tool call type injection

When processing `server_tool_use` blocks with wire name `code_execution`, the Anthropic provider SHALL check the accumulated input JSON at `content_block_stop` (streaming) or the full input object (non-streaming). If the input has a `code` field but no `type` field, the provider SHALL inject `"type": "programmatic-tool-call"` into the input.

This handles the programmatic tool calling pattern where the API sends `code_execution` tool calls with just `{code: "..."}` format, which need to be tagged with a type for downstream consumers.

#### Scenario: Streaming programmatic tool call injection

- **WHEN** a `content_block_stop` event arrives for a `server_tool_use` block with wire name `code_execution`
- **AND** the accumulated input JSON is `{"code": "print('hello')"}`
- **THEN** the emitted `PartToolCall` input is `{"type":"programmatic-tool-call","code":"print('hello')"}`

#### Scenario: Streaming code_execution with existing type field

- **WHEN** a `content_block_stop` event arrives for a `server_tool_use` block with wire name `code_execution`
- **AND** the accumulated input JSON is `{"type": "bash", "code": "ls"}`
- **THEN** the emitted `PartToolCall` input is unchanged (no injection)

#### Scenario: Non-streaming programmatic tool call injection

- **WHEN** `convertResponse` encounters a `server_tool_use` block with name `code_execution`
- **AND** the input object has a `code` field but no `type` field
- **THEN** the serialized input includes `"type": "programmatic-tool-call"` merged with the original input

#### Scenario: Streaming code_execution with malformed JSON

- **WHEN** a `content_block_stop` event arrives for a `server_tool_use` block with wire name `code_execution`
- **AND** the accumulated input JSON is malformed
- **THEN** the input is emitted as-is without injection (error silently ignored)

### Requirement: Non-streaming input wrapping for code execution sub-tools

When `convertResponse` encounters a `server_tool_use` block with wire name `bash_code_execution` or `text_editor_code_execution`, the full input SHALL be wrapped with a `type` field: `{"type": "<wireName>", ...input}`. The tool name SHALL map through `code_execution`.

#### Scenario: Non-streaming bash_code_execution input wrapping

- **WHEN** `convertResponse` encounters a `server_tool_use` block with name `bash_code_execution` and input `{"code": "ls -la"}`
- **THEN** the serialized input is `{"type":"bash_code_execution","code":"ls -la"}`
- **AND** the tool name is `mapping.toCustomToolName("code_execution")`

#### Scenario: Non-streaming text_editor_code_execution input wrapping

- **WHEN** `convertResponse` encounters a `server_tool_use` block with name `text_editor_code_execution` and input `{"command": "view", "path": "/tmp"}`
- **THEN** the serialized input is `{"type":"text_editor_code_execution","command":"view","path":"/tmp"}`
- **AND** the tool name is `mapping.toCustomToolName("code_execution")`

### Requirement: Dynamic flag for implicit code execution

The Anthropic provider SHALL compute a `markCodeExecutionDynamic` boolean after tool preparation. The flag SHALL be `true` when the prepared tools contain a tool with type `web_fetch_20260209` or `web_search_20260209` AND no tool has name `code_execution`.

When `markCodeExecutionDynamic` is `true`, all `server_tool_use` blocks with wire name `code_execution` SHALL have `Dynamic: true` set on the emitted tool call parts. This applies in both streaming (`PartToolInputStart` and `PartToolCall`) and non-streaming (`GenerateContentPart`) paths.

#### Scenario: Dynamic flag set when web_search_20260209 without code_execution

- **WHEN** the prepared tools contain a tool with `type: "web_search_20260209"` and no tool has `name: "code_execution"`
- **THEN** `markCodeExecutionDynamic` is `true`
- **AND** `server_tool_use` blocks with name `code_execution` are emitted with `Dynamic: true`

#### Scenario: Dynamic flag not set when code_execution present

- **WHEN** the prepared tools contain both a tool with `type: "web_search_20260209"` and a tool with `name: "code_execution"`
- **THEN** `markCodeExecutionDynamic` is `false`

#### Scenario: Dynamic flag not set without web 20260209 tools

- **WHEN** the prepared tools contain only `web_search_20250305` and no 20260209 web tools
- **THEN** `markCodeExecutionDynamic` is `false`

### Requirement: Code execution result block handling

The Anthropic provider SHALL handle `code_execution_tool_result` content blocks in both streaming and non-streaming paths. These blocks have three subtypes identified by their inner type field:

- `code_execution_result`: Contains `stdout`, `stderr`, `return_code`, and `content` fields
- `encrypted_code_execution_result`: Contains `encrypted_stdout`, `encrypted_stderr`, `return_code`, and `content` fields
- `code_execution_tool_result_error`: Contains `error_code` field

Each SHALL emit a `PartToolResult` (streaming) or `GenerateContentPart` with `Type: "tool-result"` (non-streaming). The tool name SHALL be `mapping.toCustomToolName("code_execution")`. The tool call ID SHALL be resolved from the `serverToolCalls` tracking map using the result's `tool_use_id`.

#### Scenario: Streaming code_execution_result with stdout

- **WHEN** a `code_execution_tool_result` content block arrives with inner type `code_execution_result` containing `stdout: "hello\n"`, `return_code: 0`
- **THEN** the adapter emits a `PartToolResult` with the result data serialized as JSON, `ToolName` set to `mapping.toCustomToolName("code_execution")`, and `ToolCallID` from the `serverToolCalls` map

#### Scenario: Non-streaming encrypted code execution result

- **WHEN** `convertResponse` encounters a `code_execution_tool_result` block with inner type `encrypted_code_execution_result`
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"` and the encrypted data serialized in `Output`

#### Scenario: Code execution error result

- **WHEN** a `code_execution_tool_result` content block arrives with inner type `code_execution_tool_result_error` and `error_code: "timeout"`
- **THEN** the emitted tool result contains the error code in its output

### Requirement: Bash code execution result block handling

The Anthropic provider SHALL handle `bash_code_execution_tool_result` content blocks in both streaming and non-streaming paths. The content SHALL be passed through as-is. The tool name SHALL be `mapping.toCustomToolName("code_execution")`.

#### Scenario: Streaming bash code execution result

- **WHEN** a `bash_code_execution_tool_result` content block arrives
- **THEN** the adapter emits a `PartToolResult` with the block's content serialized as JSON, `ToolName` set to `mapping.toCustomToolName("code_execution")`

#### Scenario: Non-streaming bash code execution result

- **WHEN** `convertResponse` encounters a `bash_code_execution_tool_result` block
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"` and the content serialized in `Output`, `ToolName` set to `mapping.toCustomToolName("code_execution")`

### Requirement: Text editor code execution result block handling

The Anthropic provider SHALL handle `text_editor_code_execution_tool_result` content blocks in both streaming and non-streaming paths. The content SHALL be passed through as-is. The tool name SHALL be `mapping.toCustomToolName("code_execution")`.

#### Scenario: Streaming text editor code execution result

- **WHEN** a `text_editor_code_execution_tool_result` content block arrives
- **THEN** the adapter emits a `PartToolResult` with the block's content serialized as JSON, `ToolName` set to `mapping.toCustomToolName("code_execution")`

#### Scenario: Non-streaming text editor code execution result

- **WHEN** `convertResponse` encounters a `text_editor_code_execution_tool_result` block
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"` and the content serialized in `Output`, `ToolName` set to `mapping.toCustomToolName("code_execution")`

### Requirement: Pre-populated input on content_block_start

When a `content_block_start` event for `tool_use` or `server_tool_use` includes a non-empty `input` object, the stream adapter SHALL pre-serialize the input into `blockState.accumulatedInput` immediately. The `firstDelta` flag SHALL be set to `true` only when the initial input is empty (indicating deltas will follow). When input is pre-populated, no `PartToolInputDelta` events are expected from streaming deltas.

#### Scenario: tool_use with pre-populated input

- **WHEN** a `content_block_start` event arrives with type `tool_use`, `id: "toolu_123"`, `name: "my_tool"`, and `input: {"key": "value"}`
- **THEN** the adapter stores `accumulatedInput: '{"key":"value"}'` in blockState
- **AND** emits `PartToolInputStart` with the tool name and ID
- **AND** when `content_block_stop` arrives, emits `PartToolCall` with the pre-serialized input

#### Scenario: server_tool_use with pre-populated input

- **WHEN** a `content_block_start` event arrives with type `server_tool_use`, `id: "stu_123"`, `name: "code_execution"`, and `input: {"code": "print('hi')"}`
- **THEN** the adapter stores the pre-serialized input in blockState
- **AND** the `firstDelta` flag is `false` (input already present)

### Requirement: Pre-populated content on message_start

When a `message_start` event includes a non-empty `content` array containing `tool_use` blocks, the stream adapter SHALL emit the full tool-input lifecycle for each block: `PartToolInputStart` -> `PartToolInputDelta` (with the serialized input) -> `PartToolInputEnd` -> `PartToolCall`. Caller metadata SHALL be extracted and attached if present.

#### Scenario: message_start with tool_use content

- **WHEN** a `message_start` event arrives with `content: [{type: "tool_use", id: "toolu_123", name: "my_tool", input: {"key": "value"}}]`
- **THEN** the adapter emits `PartToolInputStart` with `ToolName: "my_tool"`, `ToolCallID: "toolu_123"`
- **AND** emits `PartToolInputDelta` with `'{"key":"value"}'`
- **AND** emits `PartToolInputEnd`
- **AND** emits `PartToolCall` with the serialized input and caller metadata if present

#### Scenario: message_start with no content

- **WHEN** a `message_start` event arrives with `content: null` or `content: []`
- **THEN** no additional events are emitted

#### Scenario: message_start with tool_use with caller

- **WHEN** a `message_start` event arrives with a `tool_use` block that has `caller: {type: "code_execution_20250825", tool_id: "toolu_456"}`
- **THEN** the `PartToolCall` includes `ProviderMetadata` with `{"anthropic": {"caller": {"type": "code_execution_20250825", "toolId": "toolu_456"}}}`
