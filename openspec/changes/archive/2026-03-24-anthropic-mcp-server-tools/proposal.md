## Why

Teams using the Anthropic provider cannot configure MCP (Model Context Protocol) servers, which means the model cannot invoke remote MCP tools during generation. The upstream Vercel AI SDK supports this via provider options and handles both `mcp_tool_use` and `mcp_tool_result` content blocks. This blocks adoption by teams that need MCP-connected tool workflows.

## What Changes

- Add `MCPServers` field to `AnthropicOptions` for configuring MCP servers via provider options, mapped to `BetaMessageNewParams.MCPServers` in `applyProviderOptions()`
- Auto-inject `mcp-client-2025-04-04` beta header when MCP servers are present
- Handle `mcp_tool_use` content blocks in both streaming and non-streaming responses, emitting `PartToolCall` directly (no start/delta/end sequence) with `ProviderExecuted: true`, `Dynamic: true`, and `ProviderMetadata` containing `type: "mcp-tool-use"` and `serverName`
- Handle `mcp_tool_result` content blocks in both paths, emitting `PartToolResult` with `Dynamic: true` and metadata looked up from a per-request `mcpToolCalls` tracking map
- Add prompt conversion for round-tripping: `convertAssistantContent` detects MCP tool calls via `ProviderMetadata` and emits `mcp_tool_use` blocks; `convertToolContent` detects MCP tool results via a tracking set and emits `mcp_tool_result` blocks

## Capabilities

### New Capabilities

- `mcp-server-tools`: Support for Anthropic MCP server configuration via provider options and MCP tool content blocks (`mcp_tool_use`, `mcp_tool_result`) in both streaming and non-streaming responses, including prompt round-tripping for multi-step tool loops

### Modified Capabilities

_(none -- no existing spec requirements change)_

## Impact

- **anthropic/options.go**: Add `MCPServers` and related structs to `AnthropicOptions`
- **anthropic/convert_request.go**: `applyProviderOptions()` maps MCP server config to `BetaMessageNewParams.MCPServers` and injects beta header; `convertAssistantContent()` and `convertToolContent()` handle MCP block round-tripping
- **anthropic/convert_stream.go**: `handleEvent()` handles `mcp_tool_use` and `mcp_tool_result` at `content_block_start`, emitting `PartToolCall`/`PartToolResult` directly; adds `mcpToolCalls` tracking map to `streamAdapter`
- **anthropic/convert_response.go**: `convertResponse()` handles `mcp_tool_use` and `mcp_tool_result` blocks with tracking map
- **Dependencies**: No new dependencies -- anthropic-sdk-go v1.11.0 already supports all MCP types
