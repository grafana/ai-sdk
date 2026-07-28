## MODIFIED Requirements

### Requirement: web_search_tool_result streaming

The Anthropic stream adapter SHALL handle `web_search_tool_result` content blocks by emitting a `PartToolResult` with the result data, followed by a `PartSource` for each search result URL. The `ToolName` in emitted parts SHALL be resolved through the tool name mapping rather than hardcoded.

#### Scenario: Successful web search result with URLs

- **WHEN** a `web_search_tool_result` content block arrives with an array of search results
- **THEN** the adapter emits a `PartToolResult` with `ToolCallID` linking to the originating `server_tool_use`, `ToolName` set to `mapping.toCustomToolName("web_search")`, and the result array serialized as JSON in `Output`
- **AND** for each result in the array, the adapter emits a `PartSource` with `SourceType: "url"`, the result's `URL`, `Title`, and `pageAge` in provider metadata

#### Scenario: Web search error result

- **WHEN** a `web_search_tool_result` content block arrives with an error (not an array)
- **THEN** the adapter emits a `PartToolResult` with `ToolName` set to `mapping.toCustomToolName("web_search")` and the error information serialized in `Output`
- **AND** no `PartSource` events are emitted

### Requirement: web_search_tool_result non-streaming

The Anthropic response converter SHALL handle `web_search_tool_result` content blocks in non-streaming responses, producing `GenerateContentPart` entries for both the tool result and individual source citations. The `ToolName` in emitted parts SHALL be resolved through the tool name mapping rather than hardcoded.

#### Scenario: web_search_tool_result in non-streaming response

- **WHEN** `convertResponse()` encounters a `web_search_tool_result` content block with an array of results
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"` containing the serialized results, with `ToolName` set to `mapping.toCustomToolName("web_search")`
- **AND** it produces additional `GenerateContentPart` entries with `Type: "source"` for each URL in the results

### Requirement: tool_search_tool_result streaming

The Anthropic stream adapter SHALL handle `tool_search_tool_result` content blocks by emitting a `PartToolResult` with the result data serialized as JSON. The `ToolName` in emitted parts SHALL be resolved by looking up the originating `server_tool_use` block's wire name from the `serverToolCalls` tracking map, then passing it through the tool name mapping. If the tracking map has no entry, the handler SHALL fall back to checking which tool_search variant has a mapping entry.

#### Scenario: Successful tool search result

- **WHEN** a `tool_search_tool_result` content block arrives with tool references and `serverToolCalls` contains the originating wire name
- **THEN** the adapter emits a `PartToolResult` with `ToolCallID` linking to the originating `server_tool_use`, `ToolName` resolved via `mapping.toCustomToolName(serverToolCalls[toolUseID])`, and the result data serialized as JSON in `Output`

#### Scenario: Tool search error result

- **WHEN** a `tool_search_tool_result` content block arrives with an error and `serverToolCalls` contains the originating wire name
- **THEN** the adapter emits a `PartToolResult` with `ToolName` resolved via `mapping.toCustomToolName(serverToolCalls[toolUseID])` and the error information serialized in `Output`

### Requirement: tool_search_tool_result non-streaming

The Anthropic response converter SHALL handle `tool_search_tool_result` content blocks in non-streaming responses. The `ToolName` in emitted parts SHALL be resolved by looking up the originating wire name from the `serverToolCalls` tracking map, then passing it through the tool name mapping. If the tracking map has no entry, the handler SHALL fall back to checking which tool_search variant has a mapping entry.

#### Scenario: tool_search_tool_result in non-streaming response

- **WHEN** `convertResponse()` encounters a `tool_search_tool_result` content block and `serverToolCalls` contains the originating wire name
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"` containing the serialized result data, with `ToolName` resolved via `mapping.toCustomToolName(serverToolCalls[toolUseID])`

### Requirement: Generic server_tool_use streaming

The Anthropic stream adapter SHALL handle `server_tool_use` content blocks generically for ANY tool name. The handling SHALL follow the same start/delta/stop pattern as regular `tool_use` blocks, but with `ProviderExecuted` set to `true` on all emitted stream parts. The `ToolName` in emitted parts SHALL be resolved through the tool name mapping.

#### Scenario: server_tool_use block start

- **WHEN** a `content_block_start` event arrives with type `"server_tool_use"`
- **THEN** the adapter stores the raw wire name in block state, records `serverToolCalls[block.ID] = wireName`, and emits a `PartToolInputStart` stream part with the block's ID, `ToolName` set to `mapping.toCustomToolName(wireName)`, and `ProviderExecuted: true`

#### Scenario: server_tool_use input delta

- **WHEN** an `input_json_delta` arrives for a `server_tool_use` block
- **THEN** the adapter emits a `PartToolInputDelta` with the partial JSON and accumulates the input

#### Scenario: server_tool_use block stop

- **WHEN** a `content_block_stop` event arrives for a `server_tool_use` block
- **THEN** the adapter emits `PartToolInputEnd` followed by a `PartToolCall` with `ProviderExecuted: true`, the tool's call ID, mapped name, and accumulated input

#### Scenario: Unknown server tool name handled generically

- **WHEN** a `server_tool_use` block arrives with a tool name not known to the SDK (e.g., `"future_tool"`)
- **THEN** the adapter handles it identically to known server tools -- emitting `PartToolInputStart`, deltas, and `PartToolCall` with `ProviderExecuted: true`, with the unmapped name passed through

### Requirement: Generic server_tool_use non-streaming

The Anthropic response converter SHALL handle `server_tool_use` content blocks in non-streaming responses generically for ANY tool name. Each `server_tool_use` block SHALL produce a `GenerateContentPart` with `Type: "tool-call"`, the block's ID, mapped name, input, and `ProviderExecuted: true`.

#### Scenario: server_tool_use in non-streaming response

- **WHEN** `convertResponse()` encounters a content block with type `"server_tool_use"`
- **THEN** it records `serverToolCalls[block.ID] = wireName` and produces a `GenerateContentPart` with `Type: "tool-call"`, the block's `ID` as `ToolCallID`, `ToolName` set to `mapping.toCustomToolName(wireName)`, the block's `Input` serialized as JSON, and `ProviderExecuted: true`
