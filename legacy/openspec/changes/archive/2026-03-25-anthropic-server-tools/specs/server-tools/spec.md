## ADDED Requirements

### Requirement: Provider-defined tool request building

The Anthropic provider's `convertTools()` function SHALL convert `provider.Tool` entries with `Type == "provider-defined"` into the corresponding Anthropic SDK tool union variants, dispatching on the tool's `ID` field. Tools with `Type == "function"` (or empty) SHALL continue to use the existing `OfTool` path.

The following tool IDs SHALL be supported:
- `"anthropic.web_search_20250305"` -> `OfWebSearchTool20250305` with args: `maxUses`, `allowedDomains`, `blockedDomains`, `userLocation`
- `"anthropic.tool_search_bm25_20251119"` -> `OfToolSearchToolBm25_20251119`
- `"anthropic.tool_search_regex_20251119"` -> `OfToolSearchToolRegex20251119`

Unrecognized provider-defined tool IDs SHALL produce a warning (not an error) and be skipped.

#### Scenario: Web search tool with configuration

- **WHEN** `convertTools()` receives a `provider.Tool` with `Type: "provider-defined"`, `ID: "anthropic.web_search_20250305"`, and `Args` containing `maxUses`, `allowedDomains`, and `blockedDomains`
- **THEN** it produces a `BetaToolUnionParam` with `OfWebSearchTool20250305` populated, including the `MaxUses`, `AllowedDomains`, and `BlockedDomains` fields from the args

#### Scenario: Web search tool with no configuration

- **WHEN** `convertTools()` receives a `provider.Tool` with `Type: "provider-defined"`, `ID: "anthropic.web_search_20250305"`, and empty `Args`
- **THEN** it produces a `BetaToolUnionParam` with `OfWebSearchTool20250305` populated with default/zero values

#### Scenario: Tool search BM25

- **WHEN** `convertTools()` receives a `provider.Tool` with `Type: "provider-defined"` and `ID: "anthropic.tool_search_bm25_20251119"`
- **THEN** it produces a `BetaToolUnionParam` with `OfToolSearchToolBm25_20251119` populated

#### Scenario: Tool search regex

- **WHEN** `convertTools()` receives a `provider.Tool` with `Type: "provider-defined"` and `ID: "anthropic.tool_search_regex_20251119"`
- **THEN** it produces a `BetaToolUnionParam` with `OfToolSearchToolRegex20251119` populated

#### Scenario: Unrecognized provider-defined tool ID

- **WHEN** `convertTools()` receives a `provider.Tool` with `Type: "provider-defined"` and an unrecognized `ID`
- **THEN** a warning is added and the tool is skipped (not included in the output)

#### Scenario: Mixed function and provider-defined tools

- **WHEN** `convertTools()` receives a mix of `"function"` and `"provider-defined"` tools
- **THEN** both types are converted and included in the output slice

### Requirement: Generic server_tool_use streaming

The Anthropic stream adapter SHALL handle `server_tool_use` content blocks generically for ANY tool name. The handling SHALL follow the same start/delta/stop pattern as regular `tool_use` blocks, but with `ProviderExecuted` set to `true` on all emitted stream parts.

#### Scenario: server_tool_use block start

- **WHEN** a `content_block_start` event arrives with type `"server_tool_use"`
- **THEN** the adapter emits a `PartToolInputStart` stream part with the block's ID, tool name, and `ProviderExecuted: true`

#### Scenario: server_tool_use input delta

- **WHEN** an `input_json_delta` arrives for a `server_tool_use` block
- **THEN** the adapter emits a `PartToolInputDelta` with the partial JSON and accumulates the input

#### Scenario: server_tool_use block stop

- **WHEN** a `content_block_stop` event arrives for a `server_tool_use` block
- **THEN** the adapter emits `PartToolInputEnd` followed by a `PartToolCall` with `ProviderExecuted: true`, the tool's call ID, name, and accumulated input

#### Scenario: Unknown server tool name handled generically

- **WHEN** a `server_tool_use` block arrives with a tool name not known to the SDK (e.g., `"future_tool"`)
- **THEN** the adapter handles it identically to known server tools -- emitting `PartToolInputStart`, deltas, and `PartToolCall` with `ProviderExecuted: true`

### Requirement: Generic server_tool_use non-streaming

The Anthropic response converter SHALL handle `server_tool_use` content blocks in non-streaming responses generically for ANY tool name. Each `server_tool_use` block SHALL produce a `GenerateContentPart` with `Type: "tool-call"`, the block's ID, name, input, and `ProviderExecuted: true`.

#### Scenario: server_tool_use in non-streaming response

- **WHEN** `convertResponse()` encounters a content block with type `"server_tool_use"`
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-call"`, the block's `ID` as `ToolCallID`, the block's `Name` as `ToolName`, the block's `Input` serialized as JSON, and `ProviderExecuted: true`

### Requirement: web_search_tool_result streaming

The Anthropic stream adapter SHALL handle `web_search_tool_result` content blocks by emitting a `PartToolResult` with the result data, followed by a `PartSource` for each search result URL.

#### Scenario: Successful web search result with URLs

- **WHEN** a `web_search_tool_result` content block arrives with an array of search results
- **THEN** the adapter emits a `PartToolResult` with `ToolCallID` linking to the originating `server_tool_use`, `ToolName` set to `"web_search"`, and the result array serialized as JSON in `Output`
- **AND** for each result in the array, the adapter emits a `PartSource` with `SourceType: "url"`, the result's `URL`, `Title`, and `pageAge` in provider metadata

#### Scenario: Web search error result

- **WHEN** a `web_search_tool_result` content block arrives with an error (not an array)
- **THEN** the adapter emits a `PartToolResult` with the error information serialized in `Output`
- **AND** no `PartSource` events are emitted

### Requirement: web_search_tool_result non-streaming

The Anthropic response converter SHALL handle `web_search_tool_result` content blocks in non-streaming responses, producing `GenerateContentPart` entries for both the tool result and individual source citations.

#### Scenario: web_search_tool_result in non-streaming response

- **WHEN** `convertResponse()` encounters a `web_search_tool_result` content block with an array of results
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"` containing the serialized results
- **AND** it produces additional `GenerateContentPart` entries with `Type: "source"` for each URL in the results

### Requirement: tool_search_tool_result streaming

The Anthropic stream adapter SHALL handle `tool_search_tool_result` content blocks by emitting a `PartToolResult` with the result data serialized as JSON.

#### Scenario: Successful tool search result

- **WHEN** a `tool_search_tool_result` content block arrives with tool references
- **THEN** the adapter emits a `PartToolResult` with `ToolCallID` linking to the originating `server_tool_use` and the result data serialized as JSON in `Output`

#### Scenario: Tool search error result

- **WHEN** a `tool_search_tool_result` content block arrives with an error
- **THEN** the adapter emits a `PartToolResult` with the error information serialized in `Output`

### Requirement: tool_search_tool_result non-streaming

The Anthropic response converter SHALL handle `tool_search_tool_result` content blocks in non-streaming responses.

#### Scenario: tool_search_tool_result in non-streaming response

- **WHEN** `convertResponse()` encounters a `tool_search_tool_result` content block
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"` containing the serialized result data

### Requirement: Backward compatibility

Adding server tool support SHALL NOT change the behavior of existing function tool handling. The `convertTools()` function SHALL continue to produce `OfTool` for tools with `Type == "function"` or empty `Type`.

#### Scenario: Existing function tools unchanged

- **WHEN** `convertTools()` receives only `provider.Tool` entries with `Type: "function"`
- **THEN** the output is identical to the current behavior (all `OfTool` variants)

#### Scenario: Empty Type defaults to function

- **WHEN** `convertTools()` receives a `provider.Tool` with an empty `Type` field
- **THEN** it treats the tool as a function tool and produces an `OfTool` variant
