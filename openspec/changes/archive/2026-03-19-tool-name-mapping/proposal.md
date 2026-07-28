## Why

The Anthropic provider hardcodes tool names (`"web_search"`, `"tool_search"`) in emitted stream parts and content parts, diverging from upstream behavior. This causes two problems: (1) if a user has a function tool named `"web_search"`, there's an unresolvable naming conflict, and (2) tool search results are emitted with `"tool_search"` when the actual API names are `"tool_search_tool_regex"` and `"tool_search_tool_bm25"`, which is a bug. The upstream Vercel AI SDK resolves this with a bidirectional tool name mapping built at request time and applied at response time.

## What Changes

- Add a `ToolNameMapping` type that translates between provider wire names (what the Anthropic API uses, e.g. `"web_search"`) and custom/user-facing names (what the user registered, e.g. `"anthropic.web_search_20250305"`)
- Add a static provider tool names table mapping provider-defined tool IDs to their API wire names
- Build the mapping during request preparation (`buildParams`), making it available to both `DoStream` and `DoGenerate`
- Apply `toCustomToolName` in the response path: `convertResponse`, `emitWebSearchResult`, `emitToolSearchResult`, and `server_tool_use` handling -- replacing all hardcoded tool name strings
- Apply `toProviderToolName` in the request path when converting outgoing messages that contain tool results from previous turns
- Fix the existing bug where tool search results emit `"tool_search"` instead of the correct API name

## Capabilities

### New Capabilities
- `tool-name-mapping`: Bidirectional mapping between provider-defined tool IDs and API wire names, built at request time, applied at both request and response paths

### Modified Capabilities
- `server-tools`: Tool names in emitted stream parts and content parts will use user-facing mapped names instead of hardcoded provider wire names. The `ToolName` field in `PartToolResult`, `PartToolCall`, and `GenerateContentPart` for server tools will reflect the custom name the user registered, not the raw API name.

## Impact

- **anthropic/model.go**: `buildParams` returns the mapping alongside params; `DoStream` and `DoGenerate` pass it to response converters
- **anthropic/convert_response.go**: `convertResponse` accepts the mapping and uses `toCustomToolName` for all tool name emissions
- **anthropic/convert_stream.go**: `streamAdapter` holds the mapping; `handleEvent`, `emitWebSearchResult`, `emitToolSearchResult` use it
- **anthropic/convert_request.go**: Message conversion uses `toProviderToolName` for tool result messages in multi-turn conversations
- **No changes to the root `aisdk` package or `provider` package** -- this is entirely within the anthropic module
