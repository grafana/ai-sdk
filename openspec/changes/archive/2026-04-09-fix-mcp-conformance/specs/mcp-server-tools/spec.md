## MODIFIED Requirements

### Requirement: mcp_tool_result streaming

The Anthropic stream adapter SHALL handle `mcp_tool_result` content blocks at `content_block_start` by emitting a `PartToolResult` with `Dynamic: true`, the tool call ID from `tool_use_id`, the `toolName` and `providerMetadata` looked up from the `mcpToolCalls` map, the `isError` flag, and the content as raw JSON preserved from the API response. The content MUST be stored using the raw JSON bytes from the SDK's `RawJSON()` method, not by re-marshaling the SDK's union struct. The block SHALL NOT be registered in `blockState`.

#### Scenario: MCP tool result in streaming response

- **WHEN** a `content_block_start` event arrives with type `"mcp_tool_result"`, `tool_use_id: "tc_123"`, `is_error: false`, and `content: [{"type": "text", "text": "Result data"}]`
- **AND** `mcpToolCalls["tc_123"]` contains `ToolName: "get_weather"` and `ProviderMetadata` with `serverName: "weather-server"`
- **THEN** the adapter emits a `PartToolResult` with `ToolCallID: "tc_123"`, `ToolName: "get_weather"`, `Dynamic: true`, `ProviderMetadata` from the tracked tool call, and `Output.JSON` containing the raw JSON bytes `[{"type":"text","text":"Result data"}]`

#### Scenario: MCP tool result content preserves wire format

- **WHEN** a `content_block_start` event arrives with type `"mcp_tool_result"` and `content` is an array of text blocks
- **THEN** the `Output.JSON` field SHALL contain the exact JSON as received from the Anthropic API, not a re-serialization of the SDK's union type
- **AND** the JSON SHALL NOT contain SDK internal fields such as `OfBetaMCPToolResultBlockContent` or `OfString`

#### Scenario: MCP tool result with string content preserves wire format

- **WHEN** a `content_block_start` event arrives with type `"mcp_tool_result"` and `content` is a plain string `"some result"`
- **THEN** the `Output.JSON` field SHALL contain the raw JSON string `"some result"`, not `{"OfString":"some result","OfBetaMCPToolResultBlockContent":null}`

#### Scenario: MCP tool result with error

- **WHEN** a `content_block_start` event arrives with type `"mcp_tool_result"`, `tool_use_id: "tc_456"`, `is_error: true`, and `content: "Tool execution failed"`
- **AND** `mcpToolCalls["tc_456"]` contains the originating tool call info
- **THEN** the adapter emits a `PartToolResult` with `IsError: true`, `Dynamic: true`, and the error content as raw JSON in `Output`

#### Scenario: MCP tool result with no matching tool call

- **WHEN** a `content_block_start` event arrives with type `"mcp_tool_result"` and `tool_use_id` is NOT in the `mcpToolCalls` map
- **THEN** the adapter emits a `PartToolResult` with an empty `ToolName` and no `ProviderMetadata`, using the available fields from the block

### Requirement: mcp_tool_result non-streaming

The Anthropic response converter SHALL handle `mcp_tool_result` content blocks in non-streaming responses by producing a `GenerateContentPart` with `Type: "tool-result"`, `Dynamic: true`, and metadata looked up from the `mcpToolCalls` map. The content MUST be stored using the raw JSON bytes from the SDK's `RawJSON()` method, not by re-marshaling the SDK's union struct.

#### Scenario: MCP tool result in non-streaming response

- **WHEN** `convertResponse()` encounters a content block with type `"mcp_tool_result"`, `tool_use_id: "tc_789"`, `is_error: false`, and `content: [{"type": "text", "text": "Result data"}]`
- **AND** `mcpToolCalls["tc_789"]` contains the originating tool call info
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"`, `ToolCallID: "tc_789"`, `ToolName` from the tracked tool call, `Dynamic: true`, `ProviderMetadata` from the tracked tool call, and `Result` containing the raw JSON bytes `[{"type":"text","text":"Result data"}]`

#### Scenario: MCP tool result content preserves wire format in non-streaming

- **WHEN** `convertResponse()` encounters a content block with type `"mcp_tool_result"`
- **THEN** the `Result` field SHALL contain the exact JSON as received from the Anthropic API
- **AND** the JSON SHALL NOT contain SDK internal fields such as `OfBetaMCPToolResultBlockContent` or `OfString`

#### Scenario: MCP tool result with error in non-streaming

- **WHEN** `convertResponse()` encounters a content block with type `"mcp_tool_result"` and `is_error: true`
- **THEN** it produces a `GenerateContentPart` with `Type: "tool-result"`, `IsError: true`, `Dynamic: true`, and the error content as raw JSON in `Result`
