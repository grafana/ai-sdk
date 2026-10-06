## Context

The Anthropic provider currently supports 3 provider-defined tools (web_search, tool_search x2). The upstream Vercel AI SDK supports 17 provider-defined tools total, including code execution (3 versions), computer use (3 versions), text editor (4 versions), and bash (2 versions). These tools are the most complex because they require:

- Version-specific arguments and validation
- Beta headers threaded into API requests per-tool
- Input rewriting in both streaming and non-streaming response paths
- New result block types with specialized handling
- A dynamic flag for implicit code_execution calls from web tools

The current `convertProviderTool` function returns `(BetaToolUnionParam, *provider.Warning)` with no mechanism to communicate required beta headers back to the caller.

## Goals / Non-Goals

**Goals:**
- Support all 12 tool IDs from the issue (code_execution x3, computer x3, text_editor x4, bash x2)
- Thread beta headers from provider tools into API requests
- Handle 3 new result block types in both streaming and non-streaming paths
- Implement all 3 input rewriting rules matching upstream behavior
- Compute and thread the dynamic flag for implicit code_execution
- Handle pre-populated input on content_block_start and pre-populated content on message_start

**Non-Goals:**
- web_fetch tools (20250910, 20260209) -- separate change
- web_search_20260209 -- separate change
- memory_20250818 -- separate change
- Backward-incompatible changes to the provider.LanguageModel interface

## Decisions

### 1. Beta header return from convertProviderTool

**Decision**: Change the return signature of `convertProviderTool` to return `(BetaToolUnionParam, []string, *provider.Warning)`, adding a `[]string` for required beta headers.

**Alternative considered**: Pass a `map[string]bool` betaSet parameter and mutate it inside convertProviderTool. Rejected because mutation-based patterns are less idiomatic in Go for functions that already return values, and returning betas keeps the function pure.

**Alternative considered**: Return a struct `providerToolResult`. Rejected as over-engineering for three fields, and the existing pattern in the codebase uses multi-return.

The caller (`convertTools`) already collects betas in a `betaSet` map -- it will merge the returned slice into that set.

### 2. Input rewriting via blockState extensions

**Decision**: Add `firstDelta bool` and `providerToolName string` fields to the existing `blockState` struct in the stream adapter.

For `server_tool_use` blocks where the wire name is `bash_code_execution` or `text_editor_code_execution`:
- Store the original wire name as `providerToolName` on blockState
- Map through `code_execution` for the custom tool name (matching upstream behavior where these are sub-types of code_execution)
- On the first non-empty `input_json_delta`, rewrite the opening `{` to inject `{"type": "<providerToolName>",`
- Set `firstDelta = false` after rewriting

For `code_execution` blocks:
- At `content_block_stop`, parse the accumulated input JSON
- If it has a `code` field but no `type` field, inject `type: "programmatic-tool-call"`

The non-streaming path applies the same logic directly on the full input object.

### 3. Dynamic flag computation and threading

**Decision**: Implement `hasWebTool20260209WithoutCodeExecution(tools []anthropic.BetaToolUnionParam) bool` as a package-level helper. Compute the flag once after tool preparation in both `DoStream` and `DoGenerate`, then pass it to the stream adapter constructor and `convertResponse` function.

The helper checks the **prepared** Anthropic tools array (post-conversion), looking for tools with `type` matching `web_fetch_20260209` or `web_search_20260209` and no tool with `name == "code_execution"`. This matches the upstream logic exactly.

### 4. Result block handling as new switch cases

**Decision**: Add new cases to the existing content type switches in both `handleEvent` (streaming) and `convertResponse` (non-streaming).

The three new result block types:
- `code_execution_tool_result`: Has subtypes (result, encrypted result, error). Extract structured data, serialize as JSON for tool result output.
- `bash_code_execution_tool_result`: Pass-through content, tool name maps through `code_execution`.
- `text_editor_code_execution_tool_result`: Pass-through content, tool name maps through `code_execution`.

All three use `toCustomToolName("code_execution")` for the tool name, matching upstream.

### 5. Pre-populated input and content handling

**Decision**: Extend the `content_block_start` handler for both `tool_use` and `server_tool_use` to check for non-empty `input` and pre-serialize it into `blockState.accumulatedInput`. Set `firstDelta` based on whether initial input is empty.

For `message_start`, iterate over any pre-populated `content` array and emit the full tool-input lifecycle (start -> delta -> end -> call) for each `tool_use` block. This handles the programmatic tool calling pattern where the API sends complete tool calls in the initial message.

### 6. Tool definition construction per version

**Decision**: Each tool ID gets a dedicated case in `convertProviderTool` that constructs the appropriate Anthropic SDK type. Computer tools extract display dimensions from args. Text editor 20250728 extracts `maxCharacters`. Other tools have no version-specific args beyond what the SDK type carries.

Arg extraction follows the upstream `validateTypes` pattern: read from `tool.Args` map with type assertions, skip missing/invalid fields (use zero values). Full schema validation is deferred to a future change since the Go SDK types enforce structure at the type level.

## Risks / Trade-offs

- **[Large scope]** -> Mitigated by focusing only on the 12 tool IDs from the issue, deferring web_fetch/web_search_20260209/memory to separate changes.
- **[SDK type availability]** -> The Anthropic Go SDK may not have types for all 12 tool versions yet. If SDK types are missing, we may need to use raw JSON construction or wait for SDK updates. -> Investigate during implementation and escalate if blockers are found.
- **[Delta rewriting fragility]** -> The first-delta rewriting relies on the first non-empty delta starting with `{`. This matches upstream assumptions. -> Risk is low since the API format is stable, and we match the upstream behavior exactly.
- **[Pre-populated content in message_start]** -> This is a newer API pattern for programmatic tool calling. It may not be exercised by current conformance tests. -> Add explicit test coverage for this path.
