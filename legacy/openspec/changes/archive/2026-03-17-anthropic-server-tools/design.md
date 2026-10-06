## Context

The ai-sdk's Anthropic provider currently handles only `"function"` type tools. The `provider.Tool` struct already has `Type` (`"function"` / `"provider-defined"`), `ID`, and `Args` fields designed for provider-defined tools, but nothing reads them. On the response side, `convert_stream.go` and `convert_response.go` only handle `text`, `thinking`, `redacted_thinking`, `compaction`, and `tool_use` content blocks -- all server tool blocks (`server_tool_use`, `web_search_tool_result`, `tool_search_tool_result`) are silently dropped.

The upstream Vercel AI SDK handles server tools by mapping them to existing stream part types with `providerExecuted: true`, rather than introducing new types. Our Go `provider.StreamPart` struct already has `ProviderExecuted` and `Dynamic` fields, and `PartSource` for URL citations. The Anthropic Go SDK (anthropic-sdk-go) already supports all server tool union variants.

## Goals / Non-Goals

**Goals:**

- Support web_search (`web_search_20250305`) and tool_search (`tool_search_tool_bm25_20251119`, `tool_search_tool_regex_20251119`) in request building
- Parse `server_tool_use`, `web_search_tool_result`, and `tool_search_tool_result` content blocks in both streaming and non-streaming responses
- Emit `PartSource` events for web search result URLs (citations)
- Maintain wire compatibility with `@ai-sdk/react` (the frontend expects `tool-call`, `tool-result`, and `source` stream parts)
- Align with the upstream Vercel AI SDK's approach to provider-defined tools

**Non-Goals:**

- Request-side support for code_execution, computer_use, text_editor, bash, memory, web_fetch, or MCP server tools (these need tool-specific request building; the response side handles them generically via the generic `server_tool_use` handler)
- Adding user-facing tool factory functions at the root aisdk level (users construct `provider.Tool` directly or through a future helper layer)
- Supporting tool_search's deferred loading / dynamic tool discovery behavior (the tool_search result will be emitted but dynamic tool loading is out of scope)
- Beta header auto-detection for tool_search (Bedrock vs Vertex use different tool IDs; we handle direct Anthropic and Vertex only for now)
- Specific result block parsing for types beyond `web_search_tool_result` and `tool_search_tool_result` (other result types can be added incrementally)

## Decisions

### 1. Reuse existing stream part types -- no new core types needed

**Decision**: Map server tool content blocks to existing `PartToolCall`, `PartToolResult`, and `PartSource` stream parts, using `ProviderExecuted: true` to distinguish them from user-executed tools.

**Rationale**: This is exactly how the upstream TypeScript SDK handles it. The existing `provider.StreamPart` struct already has all required fields: `ProviderExecuted`, `Dynamic`, `ToolCallID`, `ToolName`, `Input`, `Output`, `Source`. Adding new part types would break the frontend's `useChat` hook which expects standard tool-related parts.

**Alternative considered**: Adding new `PartServerToolCall`, `PartWebSearchResult` etc. types. Rejected because it diverges from upstream, adds complexity in the SSE layer, and the frontend wouldn't know what to do with them.

### 2. Branch on `provider.Tool.Type` in `convertTools()`

**Decision**: Add a switch on `t.Type` in `convertTools()`. When `Type == "provider-defined"`, dispatch on `t.ID` to build the correct `BetaToolUnionParam` variant. When `Type == "function"` (or empty, for backward compatibility), use the existing `OfTool` path.

**Rationale**: This matches the upstream's `prepareTools()` pattern -- a switch on `tool.type`, then a nested switch on `tool.id` for provider tools. The `provider.Tool` struct already carries `ID` and `Args` for this purpose.

**Tool ID convention**: Use the same IDs as the upstream: `"anthropic.web_search_20250305"`, `"anthropic.tool_search_bm25_20251119"`, `"anthropic.tool_search_regex_20251119"`. This ensures future compatibility if we add tool factory helpers.

**Args mapping**:
- `web_search_20250305`: `Args["maxUses"]` -> `MaxUses`, `Args["allowedDomains"]` -> `AllowedDomains`, `Args["blockedDomains"]` -> `BlockedDomains`, `Args["userLocation"]` -> `UserLocation`
- `tool_search_bm25_20251119` / `tool_search_regex_20251119`: no args needed, just the type declaration

### 3. Whitelist-based handler for `server_tool_use` content blocks

**Decision**: Handle `server_tool_use` content blocks in `handleEvent()` (streaming) and `convertResponse()` (non-streaming) only for known tool names. Supported names: `web_search`, `web_fetch`, `code_execution`, `text_editor_code_execution`, `bash_code_execution`, `tool_search_tool_regex`, `tool_search_tool_bm25`. Unknown tool names are silently dropped.

**Rationale**: This matches the upstream Vercel AI SDK behavior exactly. The upstream checks tool names against a whitelist and silently ignores unrecognized ones. While a generic handler would be simpler, maintaining wire compatibility with the upstream is the priority -- new server tools should be added explicitly as they are in the upstream.

**Block state tracking**: Extend `blockState` to store `providerExecuted bool` so the stop handler knows to set the flag on the emitted `PartToolCall`.

**`ProviderExecuted` flag placement**: Only set on `tool-input-start` and `tool-call` stream parts, matching upstream. NOT set on `tool-input-delta` or `tool-input-end`.

### 4. Handle result blocks with type-specific parsing

**Decision**: Each result content block type (`web_search_tool_result`, `tool_search_tool_result`) is handled individually with type-specific parsing, matching the upstream pattern.

- `web_search_tool_result`: Emit `PartToolResult` (streaming) or `GenerateContentPart{Type: "tool-result"}` (non-streaming) with result data, then emit `PartSource` / source content parts for each search result URL.
- `tool_search_tool_result`: Emit `PartToolResult` / `GenerateContentPart{Type: "tool-result"}` with serialized tool references.
- Error cases: Set `IsError: true` on the result part (non-streaming). Streaming emits `PartToolResult` with error data in `Output`.

**Non-streaming tool results use `Result` field**: The `GenerateContentPart` struct has a `Result json.RawMessage` field (matching upstream V3 `LanguageModelV3ToolResult.result`) separate from the `Input` field used by tool calls.

**Non-streaming sources use flat fields**: Source content parts use `SourceType`, `URL`, and `Text` (for title) fields directly on `GenerateContentPart`, matching upstream V3 `LanguageModelV3Source`.

**Rationale**: Exact alignment with the upstream V3 type system. New result block types can be added by following the same pattern.

### 5. Add `PartToolResult` to `provider.StreamPart` emission for result blocks

**Decision**: Use `PartToolResult` for server tool result content blocks. The `Output` field on `StreamPart` (of type `ToolResultOutput`) will carry the serialized result data using `ToolOutputJSON` type.

**Rationale**: This aligns with the upstream which emits `tool-result` parts for server tool results. Our `ToolResultOutput` already supports JSON output. The `ToolCallID` on the result part links it back to the `server_tool_use` that initiated it.

### 6. Automatically add beta headers for tool_search

**Decision**: When a tool with ID `"anthropic.tool_search_bm25_20251119"` or `"anthropic.tool_search_regex_20251119"` is present, do NOT add beta headers -- these tools do not currently require any beta headers in the Anthropic API. For `web_search_20250305`, no beta header is needed either.

**Rationale**: The upstream only adds beta headers for specific tool versions that require them. The three tool IDs in our initial scope do not require beta headers. Future tool IDs (like code_execution or web_fetch) may need them and can be added following the same pattern.

## Risks / Trade-offs

**[Risk] Anthropic SDK type changes** -> The anthropic-sdk-go uses versioned tool type names (e.g., `OfWebSearchTool20250305`). If Anthropic releases a new version, we'll need to add new switch cases. Mitigation: Use the same versioned ID convention as upstream so adding new versions is mechanical.

**[Risk] Incomplete result block coverage** -> We're only adding specific parsing for `web_search_tool_result` and `tool_search_tool_result`. Other result block types (e.g., `code_execution_tool_result`, `web_fetch_tool_result`) will need code changes to parse their specific structures. Mitigation: The whitelist includes all known server tool names, so invocations work for tools beyond our result parsing scope. Adding new result types is mechanical.

**[Risk] `ToolResultOutput` may not be expressive enough** -> Web search results have structured data (URL, title, pageAge, encryptedContent per result). Serializing as JSON in `ToolOutputJSON` works but loses type safety. Mitigation: This matches the upstream approach. The structured data is consumed by the frontend which parses the JSON. We can add typed helpers later if needed.

**[Trade-off] No tool factory helpers** -> Users must construct `provider.Tool{Type: "provider-defined", ID: "anthropic.web_search_20250305", Args: ...}` manually. This is more verbose than the upstream's `anthropicTools.webSearch_20250305({...})`. Accepted because: (1) this is a provider-level concern, not blocking functionality, (2) factory functions can be added as a follow-up without changing the core design.

**[Trade-off] No tool name mapping** -> The upstream has a `toolNameMapping` system that maps between provider names (e.g., `web_search`) and custom user-facing names. We skip this for now since our Go SDK doesn't have the same customization layer. The tool name used in stream parts will be the provider's name (e.g., `web_search`).
