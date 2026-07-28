## Context

The anthropic module currently handles 15 provider-defined tool IDs in `convertProviderTool` but is missing 4 tools present in the upstream: `web_fetch_20250910`, `web_fetch_20260209`, `web_search_20260209`, and `memory_20250818`. On the response side, `web_fetch_tool_result` blocks arriving from the API are silently dropped since neither `convert_stream.go` nor `convert_response.go` handle them. The existing `web_search_tool_result` handler works for both web_search versions once tool name mapping is in place, and the `complex-server-tools` spec already defines the `markCodeExecutionDynamic` flag that depends on these v2 web tools being present.

## Goals / Non-Goals

**Goals:**
- Add request-side tool definitions for all 4 missing tool IDs with correct beta injection
- Handle `web_fetch_tool_result` in both streaming and non-streaming paths with camelCase field conversion
- Track fetched documents in `citationDocuments` for citation resolution
- Add tool name mapping entries for all 4 tools

**Non-Goals:**
- Memory tool response handling (upstream has none -- memory effects flow through `context_management`, not tool result blocks)
- Changes to the `markCodeExecutionDynamic` flag logic (already specified in `complex-server-tools`)
- New provider tool factory types or output schemas (those live in the orchestration layer, not the provider)

## Decisions

### Decision: web_fetch request building follows existing pattern

Both web_fetch versions share the same arg shape (`maxUses`, `allowedDomains`, `blockedDomains`, `citations`, `maxContentTokens`). Each gets its own case in the `convertProviderTool` switch with the appropriate Go SDK type and beta header. This mirrors the pattern used for code_execution versions.

Alternatives: Shared helper function for the two versions. Rejected because the Go SDK types are distinct union variants and the beta headers differ, so the cases would diverge anyway.

### Decision: web_search_20260209 reuses web_search_20250305 structure

The v2 web_search tool has the same request shape as v1 (args: `maxUses`, `allowedDomains`, `blockedDomains`, `userLocation`) and produces the same `web_search_tool_result` response block type. The existing response handler already works once tool name mapping resolves the name correctly. Only request-side conversion and a tool name mapping entry are needed.

### Decision: web_fetch_tool_result handler mirrors web_search_tool_result pattern

The `web_fetch_tool_result` handler will be structured as a new case in the `content_block_start` switch (streaming) and block iteration (non-streaming), following the same pattern as `web_search_tool_result`. Two subtypes:

- **Success** (`web_fetch_result`): Emit `PartToolResult` with structured JSON output containing `type`, `url`, `retrievedAt`, nested `content` (with `type`, `title`, `citations`, `source` including `type`, `mediaType`, `data`). All field names camelCased from the wire format. Then push document into `citationDocuments` with `title` (fallback to URL if nil) and `mediaType`.
- **Error** (`web_fetch_tool_result_error`): Emit `PartToolResult` with `IsError: true` and JSON `{type, errorCode}`.

Tool name resolved via `mapping.toCustomToolName("web_fetch")`. Tool call ID resolved via `serverToolCalls` tracking map.

### Decision: Citation document tracking extended to web_fetch

On successful web_fetch results, the fetched document is pushed to `citationDocuments` before emitting the tool result. This matches the upstream exactly: `{title: content.title ?? url, mediaType: source.media_type}`. The citation document list is already threaded through both streaming and non-streaming paths from the existing implementation, so no structural changes are needed -- just an additional push point.

### Decision: memory_20250818 is request-only

The memory tool definition sends `{type: "memory_20250818", name: "memory"}` with beta `context-management-2025-06-27` and has no args. No response handler is needed. This is the simplest case -- a single switch entry.

## Risks / Trade-offs

- [Go SDK type availability] The Go SDK (`anthropic-sdk-go`) needs to have the union variants for the new tool types (`OfWebFetchTool20250910`, etc.). If they're missing, we'll need to use raw JSON construction or wait for an SDK update. -> Check SDK availability before implementing; fall back to raw param construction if needed.
- [Conformance test fixtures] Test fixtures exist at `test/conformance/anthropic/upstream/web-fetch-tool/` and `web-fetch-tool-20260209/`. These should be used to validate the implementation. -> Run conformance tests after implementation.
