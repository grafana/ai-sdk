## Context

The Anthropic provider handles three layers of tool naming:
1. **Provider-defined tool IDs** (SDK-facing): `"anthropic.web_search_20250305"`, `"anthropic.tool_search_bm25_20251119"`, etc. -- used by callers of `provider.LanguageModel`
2. **API wire names** (Anthropic API-facing): `"web_search"`, `"tool_search_tool_bm25"`, `"tool_search_tool_regex"` -- what the Anthropic API sends/receives in `server_tool_use` and result blocks
3. **User custom names** (caller-facing): The `Name` field on `provider.Tool` -- what the caller chose to call the tool

Currently, the response path hardcodes wire names as `ToolName` in emitted stream parts. There is no mapping between these layers. The upstream Vercel AI SDK solves this with `createToolNameMapping()`, a bidirectional lookup built at request time.

## Goals / Non-Goals

**Goals:**
- Implement a `toolNameMapping` struct that translates between custom tool names and provider wire names
- Build the mapping during `buildParams`, making it available to both streaming and non-streaming response paths
- Replace all hardcoded tool name strings in `convert_response.go` and `convert_stream.go`
- Fix the bug where tool search results emit `"tool_search"` instead of the correct wire names (`"tool_search_tool_regex"`, `"tool_search_tool_bm25"`)
- Apply reverse mapping on the request path for tool call/result names in multi-turn conversations

**Non-Goals:**
- Adding new provider-defined tool types (code_execution, computer, bash, etc.) -- those come in future changes
- Changing the `provider.Tool` interface or the root `aisdk` package
- Conflict detection or error reporting for duplicate mappings -- follow upstream's last-wins behavior

## Decisions

### 1. Mapping struct lives in the anthropic package as an unexported type

The `toolNameMapping` is an implementation detail of the anthropic provider. It does not belong in the `provider` package. It will be a simple struct with two `map[string]string` fields and two lookup methods that fall back to identity (passthrough) for unmapped names.

**Alternative**: Put it in `provider` or a shared utility package. Rejected because no other provider needs this yet, and YAGNI applies.

### 2. Static provider tool names table as a package-level variable

A `var providerToolNames = map[string]string{...}` maps provider-defined tool IDs to their API wire names. This table covers only the tools we support today (web_search, tool_search variants). It grows as we add more provider-defined tools.

This mirrors the upstream approach where the table is defined inline in `getArgs()`.

### 3. buildParams returns the mapping as a third value

`buildParams` currently returns `(params, warnings, error)`. It will return `(params, toolNameMapping, warnings, error)`. The mapping is built from the `opts.Tools` slice and the static table in the same place where tools are converted.

**Alternative**: Build the mapping inside `convertTools` and return it alongside the tool params. Rejected because the issue explicitly calls out that the mapping is a "sibling artifact to tool preparation" and should be built alongside it, not inside it -- matching upstream's `getArgs()` pattern.

### 4. streamAdapter and convertResponse accept the mapping

- `streamAdapter` gets a `mapping toolNameMapping` field, set during construction in `consumeStream`
- `convertResponse` takes the mapping as a parameter
- Both use `mapping.toCustomToolName()` wherever they emit `ToolName`

### 5. serverToolCalls tracking map for result-to-call correlation

Both `convertResponse` and `streamAdapter` maintain a `serverToolCalls map[string]string` that maps `tool_use_id -> provider wire name`. When a `server_tool_use` block is processed, its ID and wire name are recorded. When a result block (e.g. `tool_search_tool_result`) arrives, it looks up the originating `server_tool_use` via `tool_use_id` to determine which provider wire name to map through `toCustomToolName`.

This is needed because result blocks don't carry the tool name -- only the `tool_use_id` linking back to the originating call. Without this tracking, `tool_search_tool_result` can't distinguish whether the result came from `tool_search_tool_bm25` or `tool_search_tool_regex`.

The upstream has the same pattern: a `serverToolCalls: Record<string, string>` in both `doGenerate` and `doStream`. It also includes a fallback for when the tracking map doesn't have an entry (checks which tool_search variant has a mapping and uses that). We adopt the same fallback.

### 6. Request path mapping for multi-turn tool results

`convertAssistantContent` currently passes `p.ToolName` directly to the Anthropic SDK. For provider-executed tools, the caller's custom name needs to be mapped back to the wire name. The mapping's `toProviderToolName()` method handles this.

This applies to `ToolCallContentPart` in `convertAssistantContent` where the `Name` field in `BetaToolUseBlockParam` is set.

### 7. New file for the mapping

The mapping type, constructor, and static table go in a new `tool_name_mapping.go` file within the anthropic package. This keeps the type focused and avoids bloating existing files.

## Risks / Trade-offs

- **[Risk] Mapping built per-request** -- Each call to `buildParams` creates a new mapping by iterating the tools slice. For typical tool counts (< 20), this is negligible. No mitigation needed.
- **[Risk] Last-wins on duplicate provider tool IDs** -- If a caller passes two tools mapping to the same wire name, the last one wins for the reverse lookup. This matches upstream behavior and is acceptable.
- **[Trade-off] Signature change to buildParams** -- Adding a return value changes all callers. There are only two (`DoStream`, `DoGenerate`), so the blast radius is small.
- **[Trade-off] convertResponse signature change** -- Adding the mapping parameter means updating the call site and any tests that call `convertResponse` directly. Straightforward mechanical change.
