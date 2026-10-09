# provider-executed-tool-roundtrip Specification

## Purpose

Define how provider-executed tool calls and their results round-trip through the orchestration loop (`appendToolResults`) and the Anthropic provider's request converter (`convertAssistantContent`), so server-side tools (web_search, code_execution, web_fetch, tool_search, MCP) preserve their inline placement and provider-specific block types across multi-step generation.
## Requirements
### Requirement: Orchestration routes provider-executed tool results inline in assistant message

appendToolResults SHALL build []provider.ContentPart from the step in stream order: reasoning first, nonempty text, each call immediately followed by its matching provider-executed result, then remaining non-provider-executed results. It SHALL delegate to public ToResponseMessages(parts), append returned messages to input msgs and return them. Provider results SHALL remain inline; local results SHALL use a separate tool message; provider-only results SHALL NOT append a tool message.

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

### Requirement: Tool rounds retain reasoning and provider metadata

In appendToolResults, every tool-call, tool-result and reasoning part SHALL carry ProviderMetadata through to ProviderOptions. Provider-executed inline results SHALL honor ModelOutput. Reasoning and its ProviderOptions, notably Anthropic extended-thinking signatures, SHALL survive every tool-result round.

#### Scenario: Reasoning metadata survives multiple tool rounds
- **WHEN** a step with signed reasoning and tool results is appended to history
- **THEN** reasoning ProviderOptions and all call/result metadata SHALL survive, and inline results SHALL use supplied ModelOutput.

### Requirement: Anthropic converts provider-executed tool calls to server_tool_use blocks

convertAssistantContent SHALL check ToolCallContentPart.ProviderExecuted. True non-MCP calls SHALL emit server_tool_use instead of tool_use with names resolved by toolNameMapping.toProviderToolName.

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

### Requirement: Anthropic code execution sub-tool wire names

For provider-executed code execution calls, input type bash_code_execution or text_editor_code_execution SHALL become the server_tool_use wire name. For programmatic-tool-call, conversion SHALL strip input type and emit server_tool_use named code_execution.

#### Scenario: Text editor execution uses its sub-tool name
- **WHEN** a provider-executed code_execution call has input type text_editor_code_execution
- **THEN** server_tool_use SHALL use text_editor_code_execution as its wire name.

### Requirement: Anthropic converts inline tool results to provider-specific result blocks

convertAssistantContent SHALL dispatch inline ToolResultContentPart by toolNameMapping.toProviderToolName: MCP IDs tracked in mcpToolUseIDs to mcp_tool_result; web_search/web_fetch to web_search_tool_result/web_fetch_tool_result; tool_search_tool_regex/tool_search_tool_bm25 to tool_search_tool_result. Unsupported or unrecognized results SHALL produce a warning.

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

### Requirement: Anthropic code result dispatch uses output type

Inline code_execution results SHALL dispatch to code_execution_tool_result, bash_code_execution_tool_result or text_editor_code_execution_tool_result according to the output content type field.

#### Scenario: Code result type determines the native block
- **WHEN** an inline code_execution output indicates a bash execution result
- **THEN** conversion SHALL emit bash_code_execution_tool_result rather than a generic tool result.

### Requirement: Anthropic caller metadata survives server-tool round trips
Anthropic server tool calls and web search/fetch results SHALL preserve supplied
caller metadata in generate and streaming output. Continuation SHALL project that
metadata back into native caller fields, including direct and programmatic callers.
Web-search error-json results SHALL become native web_search_tool_result_error
blocks, retaining errorCode or using unavailable when no code can be extracted.

#### Scenario: Server call and successful result retain caller
- **WHEN** a server-tool call or web search/fetch result carries caller metadata
- **THEN** normalized output carries anthropic.caller with type and optional toolId
- **AND** continuation restores native caller type and tool_id

#### Scenario: Error result retains caller
- **WHEN** a web search/fetch error result is normalized and replayed
- **THEN** the provider-specific error block and caller both survive rather than dropping the result

