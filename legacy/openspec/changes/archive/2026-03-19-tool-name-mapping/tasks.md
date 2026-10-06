## 1. Tool Name Mapping Type

- [x] 1.1 Create `anthropic/tool_name_mapping.go` with the `toolNameMapping` struct (two `map[string]string` fields), `toCustomToolName` method, `toProviderToolName` method, and the `newToolNameMapping` constructor that takes a `[]provider.Tool` slice and builds the mapping from the static table
- [x] 1.2 Define the `providerToolNames` package-level variable mapping tool IDs to wire names (`"anthropic.web_search_20250305"` -> `"web_search"`, `"anthropic.tool_search_bm25_20251119"` -> `"tool_search_tool_bm25"`, `"anthropic.tool_search_regex_20251119"` -> `"tool_search_tool_regex"`)
- [x] 1.3 Add tests in `anthropic/tool_name_mapping_test.go`: bidirectional lookup, identity passthrough for unmapped names, only provider-defined tools create entries, function-only tools produce empty mapping

## 2. Thread Mapping Through buildParams

- [x] 2.1 Change `buildParams` signature to return `(anthropic.BetaMessageNewParams, toolNameMapping, []provider.Warning, error)` and call `newToolNameMapping(opts.Tools)` to build the mapping
- [x] 2.2 Update `DoStream` and `DoGenerate` to accept the mapping from `buildParams` and pass it forward to the response path
- [x] 2.3 Update existing tests that call `buildParams` to handle the new return value

## 3. Response Path: Apply toCustomToolName

- [x] 3.1 Add a `serverToolCalls map[string]string` field to both `convertResponse` (local variable) and `streamAdapter` (struct field) to track `tool_use_id -> provider wire name` for server tool call correlation
- [x] 3.2 Change `convertResponse` to accept a `toolNameMapping` parameter; record `serverToolCalls[id] = wireName` for `server_tool_use` blocks; use `mapping.toCustomToolName()` for all tool name emissions -- replacing all hardcoded `"web_search"` and `"tool_search"` strings
- [x] 3.3 For `tool_search_tool_result` in `convertResponse`, look up the originating wire name from `serverToolCalls`, with fallback to checking which tool_search variant has a mapping entry
- [x] 3.4 Add a `mapping toolNameMapping` field to `streamAdapter` and set it in `consumeStream`
- [x] 3.5 Update `handleEvent` to use `mapping.toCustomToolName()` for `server_tool_use` block start/stop events, and record `serverToolCalls[id] = wireName` at block start
- [x] 3.6 Update `emitWebSearchResult` to use `mapping.toCustomToolName("web_search")` instead of hardcoded `"web_search"`
- [x] 3.7 Update `emitToolSearchResult` to look up the originating wire name from `serverToolCalls` (with fallback), then use `mapping.toCustomToolName()` instead of hardcoded `"tool_search"`
- [x] 3.8 Update existing response/stream tests to pass the mapping and verify mapped names, including serverToolCalls tracking

## 4. Request Path: Apply toProviderToolName

- [x] 4.1 Update `convertAssistantContent` to accept a `toolNameMapping` and use `mapping.toProviderToolName()` on `ToolCallContentPart.ToolName` when setting the `Name` field in `BetaToolUseBlockParam`
- [x] 4.2 Thread the mapping through `buildParams` to `convertAssistantContent` call sites
- [x] 4.3 Add tests verifying that custom tool names in assistant messages are mapped back to provider wire names

## 5. Verification

- [x] 5.1 Run full test suite (`make test`) and fix any failures
- [x] 5.2 Run lint/vet (`make vet`) and fix any issues
