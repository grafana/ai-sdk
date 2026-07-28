## Why

The upstream Vercel AI SDK supports web_fetch, memory, and web_search v2 as provider-defined server tools, but our Go port is missing all four tool IDs. This creates a gap in upstream compatibility -- users cannot use these tools through our provider, and web_fetch result blocks from the API are silently dropped.

## What Changes

- Add request-side tool definitions for 4 new provider tool IDs: `web_fetch_20250910`, `web_fetch_20260209`, `web_search_20260209`, and `memory_20250818`
- Add beta header injection for each: `web-fetch-2025-09-10`, `code-execution-web-tools-2026-02-09`, `context-management-2025-06-27`
- Add `web_fetch_tool_result` response handling in both streaming and non-streaming paths, including success (structured content with URL, title, source data) and error subtypes
- Add citation document tracking for web_fetch results (push fetched documents into the citation documents list for later citation resolution)
- Add tool name mapping entries for all 4 new tool IDs
- Memory tool is request-only (no response handling needed -- memory effects flow through `context_management` response fields, not tool result blocks)
- `web_search_20260209` reuses the existing `web_search_tool_result` handler (no new result handler needed)

## Capabilities

### New Capabilities

(none -- all changes fit within existing capability boundaries)

### Modified Capabilities

- `server-tools`: Adding 4 new provider tool IDs to `convertProviderTool`, adding `web_fetch_tool_result` response handling in streaming and non-streaming paths, extending citation document tracking to include web_fetch results
- `tool-name-mapping`: Adding mapping entries for `memory_20250818`, `web_search_20260209`, `web_fetch_20250910`, `web_fetch_20260209`

## Impact

- `anthropic/convert_request.go`: New cases in `convertProviderTool` switch for 4 tool IDs
- `anthropic/convert_stream.go`: New `web_fetch_tool_result` case in `content_block_start` handler, citation document push on success
- `anthropic/convert_response.go`: New `web_fetch_tool_result` case in block iteration, citation document push on success
- `anthropic/tool_name_mapping.go`: 4 new entries in `providerToolNames` map
- Dependencies: Requires #9 (tool name mapping) for `toCustomToolName` in result handlers, #10 (code execution tools) for beta header threading mechanism
