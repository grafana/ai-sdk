## Context

Multi-turn conversations with Anthropic server-side tools (web_search, code_execution, tool_search, etc.) break on the second turn. The first turn correctly streams provider-executed tool calls and results, but when the orchestration layer builds messages for the next API call, two independent bugs produce a malformed request:

1. `appendToolResults` (streamtext.go) puts ALL tool results into a separate `ToolMessage`, regardless of `ProviderExecuted`. Upstream `toResponseMessages` splits them: provider-executed results go inline in the `AssistantMessage`, non-provider-executed results go in the `ToolMessage`.

2. `convertAssistantContent` (anthropic/convert_request.go) has no handler for `ToolResultContentPart` in assistant messages. Even when results are correctly placed inline (by `ConvertToModelMessages`), the Anthropic request builder silently drops them. It also doesn't distinguish `ProviderExecuted` tool calls, emitting regular `tool_use` instead of `server_tool_use`.

The `ConvertToModelMessages` path (convert.go) already handles this correctly for UI -> model conversion. The gaps are specifically in the multi-step orchestration path and the Anthropic request builder.

## Goals / Non-Goals

**Goals:**
- Provider-executed tool results survive round-tripping through multi-turn conversations
- Match upstream `toResponseMessages` behavior in `appendToolResults`
- Match upstream `convert-to-anthropic-messages-prompt.ts` behavior in `convertAssistantContent`
- Preserve `ProviderMetadata` and `ProviderOptions` through the round-trip

**Non-Goals:**
- Changing how `ConvertToModelMessages` works (it's already correct)
- Adding new tool types or result block types
- Changing the streaming or non-streaming response parsing
- Supporting providers other than Anthropic (they'll inherit the orchestration fix; provider-specific request conversion is out of scope)

## Decisions

### Decision 1: Fix `appendToolResults` to route provider-executed results inline

`appendToolResults` will check `tr.ProviderExecuted` on each `ToolResult`. Provider-executed results will be placed as `ToolResultContentPart` entries inline in the `AssistantMessage.Content`, interleaved after their corresponding `ToolCallContentPart`. Non-provider-executed results continue going into a separate `ToolMessage`.

This matches the upstream `toResponseMessages` two-pass approach:
- Pass 1 (assistant message): text + tool calls + provider-executed tool results
- Pass 2 (tool message): only non-provider-executed tool results

The function must also carry through `ProviderMetadata` from `ToolCall` to `ToolCallContentPart.ProviderOptions` and from `ToolResult` to `ToolResultContentPart.ProviderOptions`, which is currently lost.

**Alternative considered**: Building a separate assistant content slice for provider-executed results. Rejected because the upstream interleaves them with tool calls, which is the expected format for Anthropic's API.

### Decision 2: Add `ToolResultContentPart` handling to `convertAssistantContent`

The Anthropic `convertAssistantContent` function needs a new `case provider.ToolResultContentPart` that dispatches to the appropriate Anthropic API block type based on the tool name (resolved through `toolNameMapping`). The dispatch mapping follows the upstream exactly:

| Provider tool name | Anthropic result block type |
|---|---|
| MCP tool (tracked via `mcpToolUseIDs`) | `mcp_tool_result` |
| `code_execution` | `code_execution_tool_result` (or sub-type based on result content) |
| `web_search` | `web_search_tool_result` |
| `web_fetch` | `web_fetch_tool_result` |
| `tool_search_*` | `tool_search_tool_result` |
| Unknown | warning, no block emitted |

This is the most complex part of the fix because the upstream has extensive dispatch logic (~350 lines) covering all the code execution sub-types, error variants, and content serialization formats.

The existing `convertToolContent` (which handles `ToolMessage` results) already handles `ToolResultContentPart` for `tool_result` and `mcp_tool_result` blocks. The new code in `convertAssistantContent` handles the server-side tool result variants that only appear inline in assistant messages.

### Decision 3: Emit `server_tool_use` for provider-executed tool calls

`convertAssistantContent` currently emits `OfToolUse` for all `ToolCallContentPart` entries regardless of `ProviderExecuted`. When `ProviderExecuted` is true and the tool call is not an MCP tool, it should emit `server_tool_use` blocks instead.

The tool name must be mapped back to the wire name using `toolNameMapping.toProviderToolName` (the reverse of `toCustomToolName`). For code execution sub-tools, the wire name may be `code_execution`, `bash_code_execution`, or `text_editor_code_execution` based on the input's `type` field.

### Decision 4: Carry `ProviderOptions` through `ToolResultContentPart` for provider metadata

The `ToolResult.ProviderMetadata` (orchestration type) must be converted to `ToolResultContentPart.ProviderOptions` (provider type) when building provider messages in `appendToolResults`. This allows the Anthropic request converter to access provider-specific metadata (like result sub-types) needed for correct block dispatch.

Similarly, `ToolCall.ProviderMetadata` must map to `ToolCallContentPart.ProviderOptions` in `appendToolResults`, which is also currently lost.

## Risks / Trade-offs

- **Complexity of Anthropic result dispatch**: The upstream has ~350 lines of result type dispatch logic. Porting this faithfully introduces significant code. Mitigation: port incrementally, starting with web_search and code_execution as the most common server tools, with a generic fallback for unknown types.
- **No `server_tool_result` generic block in Anthropic SDK**: The Anthropic Go SDK may not expose a generic `server_tool_result` block param type. Mitigation: check the SDK's available types; if missing, use raw JSON or the closest available type. The upstream TypeScript SDK uses specific block types per tool.
- **Test coverage for round-trip paths**: The existing integration tests (`TestE2EProviderExecutedToolFlow`) test single-turn flows but not multi-turn. Mitigation: add multi-turn test cases that verify provider-executed results survive through `appendToolResults` -> `convertAssistantContent`.
