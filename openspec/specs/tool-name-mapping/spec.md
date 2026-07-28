## Purpose

Define how Anthropic provider-defined tool names are mapped between user-facing SDK names and provider wire names across request preparation and response handling.

## Requirements

### Requirement: Bidirectional tool name mapping

The Anthropic provider SHALL implement a `toolNameMapping` that translates between user-facing custom tool names and provider API wire names. The mapping SHALL be built from the `provider.Tool` slice and a static provider tool names table. Both lookup directions SHALL return the input unchanged when no mapping exists (identity passthrough).

#### Scenario: Custom name to provider name lookup

- **WHEN** `toProviderToolName` is called with a custom name that has a mapping entry
- **THEN** it returns the corresponding provider wire name

#### Scenario: Provider name to custom name lookup

- **WHEN** `toCustomToolName` is called with a provider wire name that has a mapping entry
- **THEN** it returns the corresponding custom tool name

#### Scenario: Unmapped name passthrough (custom to provider)

- **WHEN** `toProviderToolName` is called with a name that has no mapping entry
- **THEN** it returns the input name unchanged

#### Scenario: Unmapped name passthrough (provider to custom)

- **WHEN** `toCustomToolName` is called with a name that has no mapping entry
- **THEN** it returns the input name unchanged

#### Scenario: Only provider-defined tools create mappings

- **WHEN** the tools slice contains only function tools (no `Type: "provider-defined"`)
- **THEN** the mapping is empty and all lookups pass through unchanged

### Requirement: Static provider tool names table

The Anthropic provider SHALL maintain a static table mapping provider-defined tool IDs to their Anthropic API wire names. The table SHALL include entries for all currently supported provider-defined tools.

#### Scenario: Web search tool mapping

- **WHEN** a tool with `ID: "anthropic.web_search_20250305"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"web_search"`

#### Scenario: Web search v2 tool mapping

- **WHEN** a tool with `ID: "anthropic.web_search_20260209"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"web_search"`

#### Scenario: Web fetch v1 tool mapping

- **WHEN** a tool with `ID: "anthropic.web_fetch_20250910"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"web_fetch"`

#### Scenario: Web fetch v2 tool mapping

- **WHEN** a tool with `ID: "anthropic.web_fetch_20260209"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"web_fetch"`

#### Scenario: Memory tool mapping

- **WHEN** a tool with `ID: "anthropic.memory_20250818"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"memory"`

#### Scenario: Tool search BM25 mapping

- **WHEN** a tool with `ID: "anthropic.tool_search_bm25_20251119"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"tool_search_tool_bm25"`

#### Scenario: Tool search regex mapping

- **WHEN** a tool with `ID: "anthropic.tool_search_regex_20251119"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"tool_search_tool_regex"`

#### Scenario: Code execution tool mappings

- **WHEN** a tool with `ID: "anthropic.code_execution_20250522"`, `"anthropic.code_execution_20250825"`, or `"anthropic.code_execution_20260120"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"code_execution"`

#### Scenario: Computer tool mappings

- **WHEN** a tool with `ID: "anthropic.computer_20241022"`, `"anthropic.computer_20250124"`, or `"anthropic.computer_20251124"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"computer"`

#### Scenario: Text editor 20241022 and 20250124 mappings

- **WHEN** a tool with `ID: "anthropic.text_editor_20241022"` or `"anthropic.text_editor_20250124"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"str_replace_editor"`

#### Scenario: Text editor 20250429 and 20250728 mappings

- **WHEN** a tool with `ID: "anthropic.text_editor_20250429"` or `"anthropic.text_editor_20250728"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"str_replace_based_edit_tool"`

#### Scenario: Bash tool mappings

- **WHEN** a tool with `ID: "anthropic.bash_20241022"` or `"anthropic.bash_20250124"` is in the tools slice
- **THEN** the mapping maps its custom `Name` to the wire name `"bash"`

#### Scenario: Unsupported tool ID not in table

- **WHEN** a provider-defined tool has an ID not present in the static table
- **THEN** no mapping entry is created for that tool (it is skipped)

### Requirement: Mapping built during request preparation

The tool name mapping SHALL be built in `buildParams` alongside tool conversion, and SHALL be returned to callers (`DoStream`, `DoGenerate`) for use in the response path.

#### Scenario: buildParams returns mapping

- **WHEN** `buildParams` is called with tools containing provider-defined tools
- **THEN** it returns a `toolNameMapping` with entries for each provider-defined tool whose ID is in the static table

#### Scenario: buildParams with no provider tools

- **WHEN** `buildParams` is called with only function tools
- **THEN** it returns an empty `toolNameMapping`

### Requirement: Server tool call tracking for result correlation

Both `convertResponse` and `streamAdapter` SHALL maintain a `serverToolCalls` map from `tool_use_id` to provider wire name. When a `server_tool_use` block is processed, the block's ID and wire name SHALL be recorded. When a result block arrives (e.g. `tool_search_tool_result`), the handler SHALL look up the originating wire name via `tool_use_id` to determine which name to pass to `toCustomToolName`.

When the tracking map does not contain an entry for the `tool_use_id`, the handler SHALL fall back to checking which tool_search variant has a mapping entry and use that provider wire name.

#### Scenario: tool_search_tool_result resolves name via tracking map

- **WHEN** a `tool_search_tool_result` arrives with `tool_use_id: "xyz"` and `serverToolCalls["xyz"]` is `"tool_search_tool_bm25"`
- **THEN** the emitted `ToolName` is `mapping.toCustomToolName("tool_search_tool_bm25")`

#### Scenario: tool_search_tool_result falls back when tracking map is empty

- **WHEN** a `tool_search_tool_result` arrives with a `tool_use_id` not in the tracking map and the mapping has an entry for `"tool_search_tool_regex"`
- **THEN** the emitted `ToolName` is `mapping.toCustomToolName("tool_search_tool_regex")`

#### Scenario: server_tool_use populates tracking map in streaming

- **WHEN** a `server_tool_use` block start arrives with `id: "abc"` and `name: "tool_search_tool_bm25"`
- **THEN** `serverToolCalls["abc"]` is set to `"tool_search_tool_bm25"`

#### Scenario: server_tool_use populates tracking map in non-streaming

- **WHEN** `convertResponse` encounters a `server_tool_use` block with `id: "abc"` and `name: "tool_search_tool_bm25"`
- **THEN** `serverToolCalls["abc"]` is set to `"tool_search_tool_bm25"`

### Requirement: Response path uses toCustomToolName

All tool name emissions in the response path (`convertResponse` for non-streaming, `streamAdapter.handleEvent` for streaming) SHALL use `toCustomToolName` to translate provider wire names to user-facing names. No tool name SHALL be hardcoded in the response path.

#### Scenario: server_tool_use name mapped in streaming

- **WHEN** a `server_tool_use` block arrives with name `"web_search"` and the mapping has an entry for it
- **THEN** the emitted `PartToolInputStart` and `PartToolCall` use the mapped custom name

#### Scenario: server_tool_use name mapped in non-streaming

- **WHEN** `convertResponse` encounters a `server_tool_use` block with name `"web_search"` and the mapping has an entry for it
- **THEN** the emitted `GenerateContentPart` uses the mapped custom name as `ToolName`

#### Scenario: web_search_tool_result name mapped in streaming

- **WHEN** a `web_search_tool_result` block is processed and the mapping has an entry for `"web_search"`
- **THEN** the emitted `PartToolResult` uses the mapped custom name as `ToolName`

#### Scenario: web_search_tool_result name mapped in non-streaming

- **WHEN** `convertResponse` encounters a `web_search_tool_result` and the mapping has an entry for `"web_search"`
- **THEN** the emitted `GenerateContentPart` uses the mapped custom name as `ToolName`

#### Scenario: tool_search_tool_result name mapped in streaming

- **WHEN** a `tool_search_tool_result` block is processed and a tool_search mapping exists
- **THEN** the emitted `PartToolResult` uses the mapped custom name as `ToolName`, looking up the originating server_tool_use block's wire name through the mapping

#### Scenario: tool_search_tool_result name mapped in non-streaming

- **WHEN** `convertResponse` encounters a `tool_search_tool_result` and a tool_search mapping exists
- **THEN** the emitted `GenerateContentPart` uses the mapped custom name as `ToolName`

### Requirement: Request path uses toProviderToolName

When converting outgoing messages containing tool call references from previous turns, the provider SHALL use `toProviderToolName` to translate user-facing names back to provider wire names.

#### Scenario: Tool call name mapped in assistant message

- **WHEN** `convertAssistantContent` encounters a `ToolCallContentPart` with a custom tool name that has a mapping entry
- **THEN** the `Name` field in the emitted `BetaToolUseBlockParam` uses the provider wire name

#### Scenario: Unmapped tool call name passed through

- **WHEN** `convertAssistantContent` encounters a `ToolCallContentPart` with a name that has no mapping entry
- **THEN** the `Name` field passes through unchanged
