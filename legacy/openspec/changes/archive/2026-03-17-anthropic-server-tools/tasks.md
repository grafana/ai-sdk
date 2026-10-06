## 1. Request Building -- Provider-Defined Tools

- [x] 1.1 Add `convertProviderDefinedTool()` function in `anthropic/convert_request.go` that dispatches on `provider.Tool.ID` and returns `(anthropic.BetaToolUnionParam, *provider.Warning, error)`
- [x] 1.2 Implement `anthropic.web_search_20250305` case: parse `Args` for `maxUses`, `allowedDomains`, `blockedDomains`, `userLocation` and build `OfWebSearchTool20250305`
- [x] 1.3 Implement `anthropic.tool_search_bm25_20251119` case: build `OfToolSearchToolBm25_20251119`
- [x] 1.4 Implement `anthropic.tool_search_regex_20251119` case: build `OfToolSearchToolRegex20251119`
- [x] 1.5 Add default case that returns a warning for unrecognized provider-defined tool IDs
- [x] 1.6 Update `convertTools()` to branch on `t.Type`: call `convertProviderDefinedTool()` for `"provider-defined"`, existing `OfTool` path for `"function"` or empty
- [x] 1.7 Thread warnings from `convertTools()` back through `buildParams()` return value
- [x] 1.8 Write tests for `convertTools()`: web_search with args, web_search without args, tool_search_bm25, tool_search_regex, unrecognized ID warning, mixed function + provider-defined, empty Type defaults to function

## 2. Streaming -- Generic server_tool_use Handler

- [x] 2.1 Add `providerExecuted bool` field to `blockState` struct in `anthropic/convert_stream.go`
- [x] 2.2 Add `"server_tool_use"` case in the `content_block_start` switch: create `blockState` with `providerExecuted: true`, emit `PartToolInputStart` with `ProviderExecuted: true`
- [x] 2.3 Ensure `input_json_delta` handling works for `server_tool_use` blocks (already accumulates input -- verify it flows through the existing delta path)
- [x] 2.4 Update `content_block_stop` to check `blockState.providerExecuted` and set `ProviderExecuted: true` on the emitted `PartToolCall`
- [x] 2.5 Write tests for `server_tool_use` streaming: start/delta/stop events emit correct parts with `ProviderExecuted: true`, unknown tool names handled generically

## 3. Streaming -- Result Block Handlers

- [x] 3.1 Add `"web_search_tool_result"` case in `content_block_start` switch: parse the result content, emit `PartToolResult` with serialized JSON output
- [x] 3.2 For `web_search_tool_result` success case: emit a `PartSource` per search result URL with `SourceType: "url"`, URL, title, and `pageAge` in provider metadata
- [x] 3.3 For `web_search_tool_result` error case: emit `PartToolResult` with error information
- [x] 3.4 Add `"tool_search_tool_result"` case in `content_block_start` switch: emit `PartToolResult` with serialized result data
- [x] 3.5 For `tool_search_tool_result` error case: emit `PartToolResult` with error information
- [x] 3.6 Write tests for `web_search_tool_result`: successful result with URLs and sources, error result
- [x] 3.7 Write tests for `tool_search_tool_result`: successful result, error result

## 4. Non-Streaming -- server_tool_use and Result Blocks

- [x] 4.1 Add `"server_tool_use"` case in `convertResponse()`: produce `GenerateContentPart` with `Type: "tool-call"`, ID, Name, Input, `ProviderExecuted: true`
- [x] 4.2 Add `"web_search_tool_result"` case in `convertResponse()`: produce `GenerateContentPart` with `Type: "tool-result"` and source parts for each URL
- [x] 4.3 Add `"tool_search_tool_result"` case in `convertResponse()`: produce `GenerateContentPart` with `Type: "tool-result"`
- [x] 4.4 Write tests for non-streaming: server_tool_use, web_search_tool_result with sources, tool_search_tool_result

## 5. Integration Verification

- [x] 5.1 Run `make test` to verify all existing tests still pass (backward compatibility)
- [x] 5.2 Run `make vet` and `make lint` to ensure no issues
- [x] 5.3 Verify end-to-end with a streaming response fixture containing mixed regular tool_use + server_tool_use + web_search_tool_result blocks
