## Why

Teams using our ai-sdk cannot use Anthropic server tools (web_search, tool_search) because the SDK has zero support for them -- neither in request building nor response parsing. Server tool content blocks are silently dropped. This is a gap vs the upstream Vercel AI SDK and blocks adoption by teams that need grounded web search or tool search capabilities.

## What Changes

- Support server tool definitions in request params alongside regular tools, using the existing `provider.Tool` type's `"provider-defined"` type and `ID`/`Args` fields
- Parse `server_tool_use` content blocks in streaming responses (model invoking a server tool)
- Parse `web_search_tool_result` content blocks in streaming responses (search results with URLs, titles, page ages)
- Parse `tool_search_tool_result` content blocks in streaming responses
- Emit appropriate stream parts for server tool invocations and results, including source citations
- Support server tools in non-streaming response conversion
- Wire up `convertTools()` to produce the correct Anthropic SDK tool union variants (`OfWebSearchTool20250305`, `OfToolSearchToolBm25_20251119`, `OfToolSearchToolRegex20251119`) when `provider.Tool.Type` is `"provider-defined"`

## Capabilities

### New Capabilities

- `server-tools`: Support for Anthropic server tool definitions in requests and server tool content blocks in responses (server_tool_use, web_search_tool_result, tool_search_tool_result)

### Modified Capabilities

_(none -- no existing spec requirements change)_

## Impact

- **anthropic/convert_request.go**: `convertTools()` must branch on `provider.Tool.Type` to produce server tool union variants instead of always producing `OfTool`
- **anthropic/convert_stream.go**: `handleEvent()` must handle `server_tool_use`, `web_search_tool_result`, and `tool_search_tool_result` content block types
- **anthropic/convert_response.go**: `convertResponse()` must handle the same new content block types for non-streaming
- **provider/stream_part.go**: May need new StreamPart types or may reuse existing ones (PartSource for citations, PartToolCall for server tool invocations) -- to be decided in design
- **Root aisdk package**: TextStreamPart types and SSE serialization may need updates to carry server tool data to the frontend
- **Dependencies**: No new dependencies -- the anthropic-sdk-go already supports all server tool types
