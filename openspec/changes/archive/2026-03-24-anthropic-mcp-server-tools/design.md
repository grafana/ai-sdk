## Context

The Go SDK's Anthropic provider already supports server tools (`server_tool_use`, `web_search_tool_result`, `tool_search_tool_result`) via the `anthropic-server-tools` change. The provider types (`provider.StreamPart`, `GenerateContentPart`) have `Dynamic`, `ProviderExecuted`, and `ProviderMetadata` fields. The tool name mapping system is in place. The anthropic-sdk-go v1.11.0 already includes all MCP-related types: `BetaRequestMCPServerURLDefinitionParam`, `BetaMCPToolUseBlock`, `BetaMCPToolResultBlock`, and the `MCPServers` field on `BetaMessageNewParams`.

What's missing: MCP server configuration via provider options, handling of `mcp_tool_use`/`mcp_tool_result` content blocks, and prompt round-tripping for multi-step tool loops involving MCP tools.

## Goals / Non-Goals

**Goals:**

- Configure MCP servers via `AnthropicOptions` provider options, mapped to `BetaMessageNewParams.MCPServers`
- Auto-inject `mcp-client-2025-04-04` beta header when MCP servers are configured
- Handle `mcp_tool_use` and `mcp_tool_result` content blocks in both streaming and non-streaming paths
- Emit `PartToolCall`/`PartToolResult` with `ProviderExecuted: true`, `Dynamic: true`, and appropriate `ProviderMetadata`
- Track MCP tool calls in a per-request map so `mcp_tool_result` can reference the originating tool call's name and metadata
- Round-trip MCP tool calls and results in prompt conversion for multi-step tool loops

**Non-Goals:**

- Adding new provider stream part types (reuse existing `PartToolCall`, `PartToolResult`)
- Streaming MCP tool input via start/delta/end (MCP blocks are emitted complete)
- MCP tool name mapping (MCP tool names are used directly, not through the `toolNameMapping` system)
- Validation of MCP server URLs or authorization tokens

## Decisions

### 1. MCP config goes through provider options, not tool conversion

**Decision**: Add `MCPServers` to `AnthropicOptions` and map it in `applyProviderOptions()`. The config is set on `BetaMessageNewParams.MCPServers` directly.

**Rationale**: MCP servers are a request-level configuration, not individual tools. The upstream passes them through provider options, not the tools array. This also means MCP support is independent of the tool name mapping and provider-defined tool dispatch in `convertTools()`.

**Struct design**: `MCPServer` struct with `Name`, `URL`, `AuthorizationToken`, and nested `MCPToolConfiguration` with `Enabled` and `AllowedTools`. These map to `BetaRequestMCPServerURLDefinitionParam` fields.

### 2. Emit PartToolCall directly for MCP tool use -- no start/delta/end sequence

**Decision**: When `mcp_tool_use` arrives at `content_block_start`, emit a `PartToolCall` immediately with the complete tool call data. Do NOT register a `blockState` or emit `PartToolInputStart`/`PartToolInputDelta`/`PartToolInputEnd`.

**Rationale**: The upstream TypeScript SDK does exactly this -- MCP blocks arrive complete and are emitted as a single `tool-call` event. The Go SDK's `streamtext.go` `handleToolCall` works correctly without a prior `PartToolInputStart` (confirmed by code review). Skipping the delta machinery avoids accumulating stale data if the API happens to send `input_json_delta` events for MCP blocks (which should be ignored).

**Alternative considered**: Register `blockState` and stream deltas defensively. Rejected because it adds complexity, diverges from upstream, and the orchestration layer doesn't need the delta events for MCP tools.

### 3. mcpToolCalls tracking map on streamAdapter and in convertResponse

**Decision**: Add a `mcpToolCalls` field (`map[string]mcpToolCallInfo`) to `streamAdapter` for streaming, and a local `mcpToolCalls` map in `convertResponse()` for non-streaming. Each `mcp_tool_use` stores its `toolName` and `providerMetadata`, keyed by tool call ID. Each `mcp_tool_result` looks up this info.

**Struct**: `mcpToolCallInfo` with `ToolName string` and `ProviderMetadata provider.Metadata`.

**Rationale**: Matches the upstream's `mcpToolCalls` pattern exactly. The map is scoped per-request (per-stream or per-response), so there's no concurrency concern.

### 4. ProviderMetadata carries MCP type and server name

**Decision**: Set `ProviderMetadata` to `{"anthropic": {"type": "mcp-tool-use", "serverName": "<name>"}}` on MCP tool calls. MCP tool results inherit the metadata from their originating tool call.

**Rationale**: This is exactly what the upstream does. It enables downstream consumers (like prompt conversion) to identify MCP tool calls by checking `providerMetadata.anthropic.type == "mcp-tool-use"`. The `serverName` is useful for debugging and display.

### 5. Round-trip prompt conversion using ProviderMetadata detection

**Decision**: In `convertAssistantContent()`, check `ToolCallContentPart.ProviderOptions` for `anthropic.type == "mcp-tool-use"`. When detected, emit a `BetaMCPToolUseBlockParam` instead of a regular `BetaToolUseBlockParam`, and track the tool call ID in a `mcpToolUseIDs` set. In `convertToolContent()`, check if the tool result's `ToolCallID` is in `mcpToolUseIDs` and emit a `BetaRequestMCPToolResultBlockParam` instead of a regular `BetaToolResultBlockParam`.

**Challenge**: The `mcpToolUseIDs` set needs to be shared between `convertAssistantContent()` and `convertToolContent()`. Since these are called in sequence from `buildParams()`, pass the set through or return it from the assistant conversion.

**Rationale**: Multi-step tool loops send previous messages back to the model. If MCP tool calls/results are sent as regular `tool_use`/`tool_result` blocks, the API may reject them or behave unexpectedly. The upstream uses the same detection pattern.

### 6. Beta header auto-injection

**Decision**: In `applyProviderOptions()`, after parsing `MCPServers`, if the slice is non-empty, append `"mcp-client-2025-04-04"` to `p.Betas` using the existing `appendBetaUnique()` helper.

**Rationale**: This uses the same beta injection mechanism as thinking and effort. The beta header is required by the Anthropic API when MCP servers are configured.

### 7. MCP tool result content handling

**Decision**: The `mcp_tool_result` content field (which can be a string or array of text blocks) is passed through as-is by serializing it to JSON and storing it in `ToolResultOutput.JSON` (streaming) or `GenerateContentPart.Result` (non-streaming).

**Rationale**: The upstream passes `part.content` through directly without transformation. Serializing to JSON preserves the full content structure for downstream consumers.

## Risks / Trade-offs

**[Risk] MCP content_block_start may evolve to stream input** -> If future API versions add delta streaming for MCP blocks, our direct emission approach would need updating. Mitigation: This would be a new API behavior requiring changes in the upstream too, so we'd track it there.

**[Risk] mcpToolCalls map may not have entry for orphaned mcp_tool_result** -> If `mcp_tool_result` arrives without a prior `mcp_tool_use` (malformed response), the map lookup fails. Mitigation: Add a nil check and skip orphaned results with a warning, matching defensive upstream patterns.

**[Trade-off] No MCP tool name mapping** -> MCP tool names bypass the `toolNameMapping` system and are used directly. This matches upstream behavior but means MCP tools can't be aliased. Accepted because MCP tool names are dynamic and not known at tool registration time.

**[Trade-off] mcpToolUseIDs sharing between conversion functions** -> The round-trip conversion requires passing state between `convertAssistantContent()` and `convertToolContent()`. This adds a parameter to one or both functions. Accepted because the alternative (scanning the entire prompt for MCP markers) is more complex and error-prone.
