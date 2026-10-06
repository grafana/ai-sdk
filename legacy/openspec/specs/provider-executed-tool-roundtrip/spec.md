# provider-executed-tool-roundtrip Specification

## Purpose

Define how provider-executed tool calls and their results round-trip through the orchestration loop (`appendToolResults`) and the Anthropic provider's request converter (`convertAssistantContent`), so server-side tools (web_search, code_execution, web_fetch, tool_search, MCP) preserve their inline placement and provider-specific block types across multi-step generation.
## Requirements
### Requirement: Orchestration routes provider-executed tool results inline in assistant message

The `appendToolResults` function SHALL build a `[]provider.ContentPart` from
the step in stream order (reasoning blocks first, then a `text` part if
non-empty, then for each tool call the call followed immediately by its
matching provider-executed `tool-result`, then any remaining
non-provider-executed `tool-result` parts) and SHALL delegate the conversion
of that slice to the public `ToResponseMessages(parts)` helper. The
returned messages SHALL be appended to the input `msgs` slice and returned.
The function SHALL preserve all behavior previously specified —
provider-executed tool results inline in the assistant message,
non-provider-executed tool results in a separate tool message,
`ProviderMetadata` carried through to `ProviderOptions` on every
`tool-call`, `tool-result`, and `reasoning` part, and `ModelOutput` honored
for provider-executed inline results — with the additional guarantee that
`reasoning` parts and their `ProviderOptions` (notably Anthropic's extended-
thinking signature) survive every tool-result round.

When a step contains only provider-executed tool results and no
non-provider-executed tool results, no tool message SHALL be appended.

#### Scenario: Provider-executed tool result goes inline in assistant message

- **WHEN** `appendToolResults` processes a step with a tool call that has
  `ProviderExecuted: true` and a corresponding tool result with
  `ProviderExecuted: true`
- **THEN** the tool result SHALL appear as a `tool-result` `ContentPart`
  inline in the assistant message's `Content` after the `tool-call`
  `ContentPart`
- **AND** no tool message SHALL be appended for that result

#### Scenario: Non-provider-executed tool result goes in tool message

- **WHEN** `appendToolResults` processes a step with a tool call that has
  `ProviderExecuted: false` and a corresponding tool result with
  `ProviderExecuted: false`
- **THEN** the tool result SHALL appear in a separate tool message
- **AND** the tool result SHALL NOT appear inline in the assistant message's
  `Content`

#### Scenario: Mixed provider-executed and non-provider-executed results

- **WHEN** `appendToolResults` processes a step with both provider-executed
  and non-provider-executed tool calls and results
- **THEN** provider-executed results SHALL be inline in the assistant
  message's `Content`
- **AND** non-provider-executed results SHALL be in a separate tool message
- **AND** the assistant message SHALL contain tool calls for both types and
  inline results only for provider-executed tools

#### Scenario: ProviderMetadata carried through to tool-call ContentPart

- **WHEN** `appendToolResults` processes a step with a `ToolCall` that has
  non-nil `ProviderMetadata`
- **THEN** the resulting `tool-call` `ContentPart` SHALL have
  `ProviderOptions` populated from the `ToolCall.ProviderMetadata`

#### Scenario: ProviderMetadata carried through to tool-result ContentPart

- **WHEN** `appendToolResults` processes a step with a `ToolResult` that
  has non-nil `ProviderMetadata` and `ProviderExecuted: true`
- **THEN** the resulting `tool-result` `ContentPart` inline in the assistant
  message SHALL have `ProviderOptions` populated from the
  `ToolResult.ProviderMetadata`

#### Scenario: ModelOutput preserved for provider-executed inline results

- **WHEN** `appendToolResults` processes a provider-executed tool result
  that has a non-nil `ModelOutput`
- **THEN** the `tool-result` `ContentPart`'s `Output` SHALL use the
  `ModelOutput` value instead of constructing output from the raw `Output`
  JSON

#### Scenario: Only provider-executed results produces no tool message

- **WHEN** `appendToolResults` processes a step where ALL tool results have
  `ProviderExecuted: true`
- **THEN** no tool message SHALL be appended to the result

#### Scenario: Reasoning content survives across tool-result rounds

- **WHEN** `appendToolResults` processes a step that has one reasoning
  block in `step.Reasoning` carrying `ProviderMetadata` with an Anthropic
  `signature`, plus a `tool-call` and a `tool-result`
- **THEN** the appended assistant message SHALL contain a `reasoning`
  `ContentPart` whose `ProviderOptions` carries the signature, ordered
  before the `tool-call` part
- **AND** the resulting message list SHALL be suitable as the next-call
  prompt without any reasoning content being dropped

### Requirement: Anthropic converts provider-executed tool calls to server_tool_use blocks

The `convertAssistantContent` function SHALL check `ProviderExecuted` on `ToolCallContentPart` entries. When `ProviderExecuted` is true and the tool call is not an MCP tool, the function SHALL emit a `server_tool_use` block instead of a regular `tool_use` block. The tool name SHALL be resolved through `toolNameMapping.toProviderToolName`.

For code execution tool calls where the input contains a `type` field with value `bash_code_execution` or `text_editor_code_execution`, the emitted `server_tool_use` block SHALL use the sub-tool name from the input's `type` field as the wire name.

For code execution tool calls where the input contains a `type` field with value `programmatic-tool-call`, the function SHALL strip the `type` field from the input and emit a `server_tool_use` block with name `code_execution`.

#### Scenario: Provider-executed web_search tool call emits server_tool_use

- **WHEN** `convertAssistantContent` encounters a `ToolCallContentPart` with `ProviderExecuted: true` and tool name mapping to `web_search`
- **THEN** it emits a `server_tool_use` block with the provider tool name and the tool call's input

#### Scenario: Provider-executed code_execution tool call emits server_tool_use

- **WHEN** `convertAssistantContent` encounters a `ToolCallContentPart` with `ProviderExecuted: true` and tool name mapping to `code_execution`
- **THEN** it emits a `server_tool_use` block with name `code_execution`

#### Scenario: Provider-executed bash_code_execution sub-tool

- **WHEN** `convertAssistantContent` encounters a `ToolCallContentPart` with `ProviderExecuted: true`, tool name mapping to `code_execution`, and input containing `{"type": "bash_code_execution", "code": "ls"}`
- **THEN** it emits a `server_tool_use` block with name `bash_code_execution` and the full input

#### Scenario: Provider-executed programmatic-tool-call type stripped

- **WHEN** `convertAssistantContent` encounters a `ToolCallContentPart` with `ProviderExecuted: true`, tool name mapping to `code_execution`, and input containing `{"type": "programmatic-tool-call", "code": "print('hi')"}`
- **THEN** it emits a `server_tool_use` block with name `code_execution` and input `{"code": "print('hi')"}` (type field stripped)

#### Scenario: Provider-executed tool_search emits server_tool_use

- **WHEN** `convertAssistantContent` encounters a `ToolCallContentPart` with `ProviderExecuted: true` and tool name mapping to `tool_search_tool_regex` or `tool_search_tool_bm25`
- **THEN** it emits a `server_tool_use` block with the provider tool name

#### Scenario: MCP tool call still emits mcp_tool_use regardless of ProviderExecuted

- **WHEN** `convertAssistantContent` encounters a `ToolCallContentPart` with `ProviderExecuted: true` and MCP provider options
- **THEN** it emits an `mcp_tool_use` block (existing behavior unchanged)

#### Scenario: Non-provider-executed tool call still emits tool_use

- **WHEN** `convertAssistantContent` encounters a `ToolCallContentPart` with `ProviderExecuted: false`
- **THEN** it emits a regular `tool_use` block (existing behavior unchanged)

#### Scenario: Unknown provider-executed tool name produces warning

- **WHEN** `convertAssistantContent` encounters a `ToolCallContentPart` with `ProviderExecuted: true` and a provider tool name that is not recognized (not `code_execution`, `web_search`, `web_fetch`, `tool_search_*`)
- **THEN** it produces a warning and does not emit a block for that tool call

### Requirement: Anthropic converts inline tool results to provider-specific result blocks

The `convertAssistantContent` function SHALL handle `ToolResultContentPart` entries that appear inline in assistant message content. The function SHALL dispatch to the appropriate Anthropic API result block type based on the tool name resolved through `toolNameMapping.toProviderToolName`:

- MCP tool results (tracked via `mcpToolUseIDs`): `mcp_tool_result`
- `code_execution`: dispatch to `code_execution_tool_result`, `bash_code_execution_tool_result`, or `text_editor_code_execution_tool_result` based on the output content's `type` field
- `web_search`: `web_search_tool_result`
- `web_fetch`: `web_fetch_tool_result`
- `tool_search_tool_regex` / `tool_search_tool_bm25`: `tool_search_tool_result`

Unsupported or unrecognized tool results SHALL produce a warning.

#### Scenario: MCP tool result inline emits mcp_tool_result

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` whose `ToolCallID` is in the `mcpToolUseIDs` set
- **THEN** it emits an `mcp_tool_result` block with the tool result content

#### Scenario: web_search tool result inline emits web_search_tool_result

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` with tool name mapping to `web_search`
- **THEN** it emits a `web_search_tool_result` block with the result content deserialized from the output

#### Scenario: web_fetch tool result inline emits web_fetch_tool_result

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` with tool name mapping to `web_fetch`
- **THEN** it emits a `web_fetch_tool_result` block with the result content

#### Scenario: web_fetch error result emits web_fetch_tool_result with error

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` with tool name mapping to `web_fetch` and output type is error
- **THEN** it emits a `web_fetch_tool_result` block with error content including the error code

#### Scenario: code_execution_result inline emits code_execution_tool_result

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` with tool name mapping to `code_execution` and the output value has `type: "code_execution_result"`
- **THEN** it emits a `code_execution_tool_result` block with stdout, stderr, return_code, and content fields

#### Scenario: encrypted_code_execution_result inline emits code_execution_tool_result

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` with tool name mapping to `code_execution` and the output value has `type: "encrypted_code_execution_result"`
- **THEN** it emits a `code_execution_tool_result` block with the encrypted content fields

#### Scenario: bash_code_execution_result inline emits bash_code_execution_tool_result

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` with tool name mapping to `code_execution` and the output value has `type: "bash_code_execution_result"` or `type: "bash_code_execution_tool_result_error"`
- **THEN** it emits a `bash_code_execution_tool_result` block

#### Scenario: text_editor code execution result inline emits text_editor_code_execution_tool_result

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` with tool name mapping to `code_execution` and the output value has a type indicating a text editor result
- **THEN** it emits a `text_editor_code_execution_tool_result` block

#### Scenario: code_execution error result inline

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` with tool name mapping to `code_execution` and the output indicates an error with `type: "code_execution_tool_result_error"`
- **THEN** it emits a `code_execution_tool_result` block with the error content

#### Scenario: tool_search result inline emits tool_search_tool_result

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` with tool name mapping to `tool_search_tool_regex` or `tool_search_tool_bm25`
- **THEN** it emits a `tool_search_tool_result` block with the deserialized tool references

#### Scenario: Unrecognized inline tool result produces warning

- **WHEN** `convertAssistantContent` encounters a `ToolResultContentPart` with a provider tool name not matching any known server tool type
- **THEN** it produces a warning and does not emit a block for that result

