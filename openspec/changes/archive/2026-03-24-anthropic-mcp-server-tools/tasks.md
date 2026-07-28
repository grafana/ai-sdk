## 1. MCP Server Configuration

- [x] 1.1 Add `MCPServer` and `MCPToolConfiguration` structs to `anthropic/options.go`
- [x] 1.2 Add `MCPServers []MCPServer` field to `AnthropicOptions`
- [x] 1.3 Map `MCPServers` to `BetaRequestMCPServerURLDefinitionParam` slice in `applyProviderOptions()`, including `AuthorizationToken` and `ToolConfiguration` sub-fields
- [x] 1.4 Auto-inject `mcp-client-2025-04-04` beta header via `appendBetaUnique()` when `MCPServers` is non-empty
- [x] 1.5 Write tests for `applyProviderOptions()`: single server, multiple servers, minimal fields, no servers (no beta), beta dedup

## 2. Streaming -- mcp_tool_use and mcp_tool_result

- [x] 2.1 Add `mcpToolCallInfo` struct and `mcpToolCalls map[string]mcpToolCallInfo` field to `streamAdapter`
- [x] 2.2 Initialize `mcpToolCalls` map in `consumeStream()`
- [x] 2.3 Add `"mcp_tool_use"` case in `content_block_start` switch: call `AsMCPToolUse()`, serialize input, emit `PartToolCall` with `ProviderExecuted: true`, `Dynamic: true`, and `ProviderMetadata`, store in `mcpToolCalls`
- [x] 2.4 Add `"mcp_tool_result"` case in `content_block_start` switch: call `AsMCPToolResult()`, look up `mcpToolCalls`, serialize content, emit `PartToolResult` with `Dynamic: true`, metadata from tracking map, and `IsError` flag
- [x] 2.5 Handle missing `mcpToolCalls` entry gracefully (emit with empty tool name and no metadata)
- [x] 2.6 Write tests for `mcp_tool_use` streaming: correct PartToolCall fields, ProviderMetadata structure, no PartToolInputStart emitted
- [x] 2.7 Write tests for `mcp_tool_result` streaming: success case, error case, missing tracking entry

## 3. Non-Streaming -- mcp_tool_use and mcp_tool_result

- [x] 3.1 Add `mcpToolCalls` local map in `convertResponse()`
- [x] 3.2 Add `"mcp_tool_use"` case in `convertResponse()` block loop: call `AsMCPToolUse()`, produce `GenerateContentPart` with `ProviderExecuted: true`, `Dynamic: true`, `ProviderMetadata`, store in `mcpToolCalls`
- [x] 3.3 Add `"mcp_tool_result"` case in `convertResponse()` block loop: call `AsMCPToolResult()`, look up `mcpToolCalls`, produce `GenerateContentPart` with `Dynamic: true`, metadata from tracking map
- [x] 3.4 Write tests for non-streaming: `mcp_tool_use`, `mcp_tool_result` success, `mcp_tool_result` error, mixed regular + MCP blocks

## 4. Prompt Round-Trip Conversion

- [x] 4.1 Update `buildParams()` to create a `mcpToolUseIDs` set (or return it from `convertAssistantContent`)
- [x] 4.2 Update `convertAssistantContent()` to detect MCP tool calls via `ProviderOptions["anthropic"].type == "mcp-tool-use"` and emit `OfMCPToolUse` blocks, tracking IDs in the set
- [x] 4.3 Update `convertToolContent()` to accept the `mcpToolUseIDs` set and emit `OfMCPToolResult` blocks for matching tool results
- [x] 4.4 Write tests for round-trip: MCP tool call in assistant message, MCP tool result in tool message, regular tools unaffected, mixed MCP + regular

## 5. Integration Verification

- [x] 5.1 Run `make test` to verify all existing tests still pass
- [x] 5.2 Run `make vet` and `make lint` to ensure no issues
- [x] 5.3 ~~Add cross-language integration test~~ Not needed — MCP reuses existing `PartToolCall`/`PartToolResult` types with no wire format changes. Added `TestStreamAdapter_MCPFullSequence` as an in-package integration-style test instead.
