## ADDED Requirements

### Requirement: MCP server configuration via provider options

The Anthropic provider SHALL support configuring MCP servers via the `MCPServers` field in `AnthropicOptions`. Each MCP server entry SHALL have `Name` (string), `URL` (string), optional `AuthorizationToken` (string), and optional `ToolConfiguration` with `Enabled` (bool) and `AllowedTools` (string slice). The config SHALL be mapped to `BetaMessageNewParams.MCPServers` in `applyProviderOptions()`.

#### Scenario: Single MCP server with all fields

- **WHEN** `AnthropicOptions` contains one `MCPServer` with `Name: "my-server"`, `URL: "https://mcp.example.com"`, `AuthorizationToken: "token123"`, and `ToolConfiguration` with `Enabled: true` and `AllowedTools: ["tool_a", "tool_b"]`
- **THEN** `applyProviderOptions()` sets `p.MCPServers` to a single `BetaRequestMCPServerURLDefinitionParam` with `Name: "my-server"`, `URL: "https://mcp.example.com"`, `AuthorizationToken: "token123"`, and `ToolConfiguration` with `Enabled: true` and `AllowedTools: ["tool_a", "tool_b"]`

#### Scenario: Multiple MCP servers

- **WHEN** `AnthropicOptions` contains two MCP server entries
- **THEN** `applyProviderOptions()` sets `p.MCPServers` to a slice of two `BetaRequestMCPServerURLDefinitionParam` entries

#### Scenario: MCP server with minimal fields

- **WHEN** `AnthropicOptions` contains one `MCPServer` with only `Name` and `URL` (no `AuthorizationToken`, no `ToolConfiguration`)
- **THEN** `applyProviderOptions()` sets `p.MCPServers` with the name and URL, and the optional fields are omitted/zero

#### Scenario: No MCP servers configured

- **WHEN** `AnthropicOptions` has no `MCPServers` (nil or empty)
- **THEN** `applyProviderOptions()` does NOT set `p.MCPServers` and does NOT inject the MCP beta header

### Requirement: MCP beta header auto-injection

The Anthropic provider SHALL automatically inject the `mcp-client-2025-04-04` beta header when MCP servers are configured. The header SHALL be added via the existing `appendBetaUnique()` mechanism to avoid duplicates.

#### Scenario: Beta header injected when MCP servers present

- **WHEN** `AnthropicOptions` contains at least one `MCPServer` entry
- **THEN** `applyProviderOptions()` appends `"mcp-client-2025-04-04"` to `p.Betas`

#### Scenario: Beta header not duplicated

- **WHEN** `AnthropicOptions` contains MCP servers AND `Betas` already includes `"mcp-client-2025-04-04"`
- **THEN** `applyProviderOptions()` does NOT add a duplicate entry

### Requirement: mcp_tool_use streaming

The Anthropic stream adapter SHALL handle `mcp_tool_use` content blocks at `content_block_start` by emitting a `PartToolCall` directly with `ProviderExecuted: true`, `Dynamic: true`, the tool call ID, name, serialized input, and `ProviderMetadata` containing `{"anthropic": {"type": "mcp-tool-use", "serverName": "<server_name>"}}`. The block SHALL NOT be registered in `blockState` and no `PartToolInputStart`/delta/end events SHALL be emitted. The tool call SHALL be tracked in the `mcpToolCalls` map.

#### Scenario: MCP tool use in streaming response

- **WHEN** a `content_block_start` event arrives with type `"mcp_tool_use"`, `id: "tc_123"`, `name: "get_weather"`, `server_name: "weather-server"`, and `input: {"city": "London"}`
- **THEN** the adapter emits a `PartToolCall` with `ToolCallID: "tc_123"`, `ToolName: "get_weather"`, `Input: "{\"city\":\"London\"}"`, `ProviderExecuted: true`, `Dynamic: true`, and `ProviderMetadata: {"anthropic": {"type": "mcp-tool-use", "serverName": "weather-server"}}`
- **AND** the adapter stores the tool call info in `mcpToolCalls["tc_123"]`

#### Scenario: MCP tool use does not emit start/delta/end

- **WHEN** a `content_block_start` event arrives with type `"mcp_tool_use"`
- **THEN** no `PartToolInputStart`, `PartToolInputDelta`, or `PartToolInputEnd` events are emitted
- **AND** subsequent `input_json_delta` or `content_block_stop` events for that block index are ignored (block not in `blockState`)

### Requirement: mcp_tool_result streaming

The Anthropic stream adapter SHALL handle `mcp_tool_result` content blocks at `content_block_start` by emitting a `PartToolResult` with `Dynamic: true`, the tool call ID from `tool_use_id`, the `toolName` and `providerMetadata` looked up from the `mcpToolCalls` map, the `isError` flag, and the content serialized as JSON. The block SHALL NOT be registered in `blockState`.

#### Scenario: MCP tool result in streaming response

- **WHEN** a `content_block_start` event arrives with type `"mcp_tool_result"`, `tool_use_id: "tc_123"`, `is_error: false`, and `content: "Weather is sunny"`
- **AND** `mcpToolCalls["tc_123"]` contains `ToolName: "get_weather"` and `ProviderMetadata` with `serverName: "weather-server"`
- **THEN** the adapter emits a `PartToolResult` with `ToolCallID: "tc_123"`, `ToolName: "get_weather"`, `Dynamic: true`, `ProviderMetadata` from the tracked tool call, and `Output` containing the serialized content

#### Scenario: MCP tool result with error

- **WHEN** a `content_block_start` event arrives with type `"mcp_tool_result"`, `tool_use_id: "tc_456"`, `is_error: true`, and `content: "Tool execution failed"`
- **AND** `mcpToolCalls["tc_456"]` contains the originating tool call info
- **THEN** the adapter emits a `PartToolResult` with `IsError: true`, `Dynamic: true`, and the error content serialized in `Output`

#### Scenario: MCP tool result with no matching tool call

- **WHEN** a `content_block_start` event arrives with type `"mcp_tool_result"` and `tool_use_id` is NOT in the `mcpToolCalls` map
- **THEN** the adapter emits a `PartToolResult` with an empty `ToolName` and no `ProviderMetadata`, using the available fields from the block

### Requirement: mcp_tool_use non-streaming

The Anthropic response converter SHALL handle `mcp_tool_use` content blocks in non-streaming responses by producing a `GenerateContentPart` with `Type: "tool-call"`, `ProviderExecuted: true`, `Dynamic: true`, and `ProviderMetadata` containing `{"anthropic": {"type": "mcp-tool-use", "serverName": "<server_name>"}}`. The tool call SHALL be tracked in a local `mcpToolCalls` map.

#### Scenario: MCP tool use in non-streaming response

- **WHEN** `convertResponse()` encounters a content block with type `"mcp_tool_use"`, `id: "tc_789"`, `name: "search_docs"`, `server_name: "docs-server"`, and `input: {"query": "hello"}`
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-call"`, `ToolCallID: "tc_789"`, `ToolName: "search_docs"`, `Input` containing the serialized input, `ProviderExecuted: true`, `Dynamic: true`, and `ProviderMetadata: {"anthropic": {"type": "mcp-tool-use", "serverName": "docs-server"}}`
- **AND** the tool call is stored in `mcpToolCalls["tc_789"]`

### Requirement: mcp_tool_result non-streaming

The Anthropic response converter SHALL handle `mcp_tool_result` content blocks in non-streaming responses by producing a `GenerateContentPart` with `Type: "tool-result"`, `Dynamic: true`, and metadata looked up from the `mcpToolCalls` map.

#### Scenario: MCP tool result in non-streaming response

- **WHEN** `convertResponse()` encounters a content block with type `"mcp_tool_result"`, `tool_use_id: "tc_789"`, `is_error: false`, and `content: [{"type": "text", "text": "Result data"}]`
- **AND** `mcpToolCalls["tc_789"]` contains the originating tool call info
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"`, `ToolCallID: "tc_789"`, `ToolName` from the tracked tool call, `Dynamic: true`, `ProviderMetadata` from the tracked tool call, and `Result` containing the serialized content

#### Scenario: MCP tool result with error in non-streaming

- **WHEN** `convertResponse()` encounters a content block with type `"mcp_tool_result"` and `is_error: true`
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"`, `IsError: true`, `Dynamic: true`, and the error content serialized in `Result`

### Requirement: MCP tool call round-trip in prompt conversion

The Anthropic request builder SHALL convert MCP tool calls and results back to their correct API block types when they appear in multi-step prompts. `ToolCallContentPart` entries with `ProviderOptions` containing `anthropic.type == "mcp-tool-use"` SHALL be emitted as `BetaMCPToolUseBlockParam`. `ToolResultContentPart` entries whose `ToolCallID` matches a tracked MCP tool call SHALL be emitted as `BetaRequestMCPToolResultBlockParam`.

#### Scenario: MCP tool call in assistant message

- **WHEN** `convertAssistantContent()` encounters a `ToolCallContentPart` with `ProviderOptions` containing `{"anthropic": {"type": "mcp-tool-use", "serverName": "my-server"}}`
- **THEN** it emits a `BetaContentBlockParamUnion` with `OfMCPToolUse` set, containing the tool call ID, name, input, and server name
- **AND** the tool call ID is added to a `mcpToolUseIDs` set

#### Scenario: MCP tool result in tool message

- **WHEN** `convertToolContent()` encounters a `ToolResultContentPart` whose `ToolCallID` is in the `mcpToolUseIDs` set
- **THEN** it emits a `BetaContentBlockParamUnion` with `OfMCPToolResult` set, containing the tool use ID, is_error flag, and content

#### Scenario: Regular tool call not converted to MCP

- **WHEN** `convertAssistantContent()` encounters a `ToolCallContentPart` WITHOUT `anthropic.type == "mcp-tool-use"` in `ProviderOptions`
- **THEN** it emits a regular `BetaToolUseBlockParam` as before (existing behavior unchanged)

#### Scenario: Regular tool result not converted to MCP

- **WHEN** `convertToolContent()` encounters a `ToolResultContentPart` whose `ToolCallID` is NOT in the `mcpToolUseIDs` set
- **THEN** it emits a regular `BetaToolResultBlockParam` as before (existing behavior unchanged)
