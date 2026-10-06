## 1. Record conformance test (TDD target)

- [x] 1.1 Create `test/conformance/anthropic/recorded/multi-step-web-search/config.yaml` with `stopWhenStepCount: 2`, a web_search provider tool, and a prompt that triggers a search followed by a text response (the second step uses the conversation history including the provider-executed tool result)
- [x] 1.2 Record the fixture from the real Anthropic API (`cd test/conformance/tools && npx tsx record.mts --scenario multi-step-web-search`) -- this captures `input-1.chunks.txt` (step 1: model calls web_search, gets results) and `input-2.chunks.txt` (step 2: model generates text using the search results from history)
- [x] 1.3 Generate `expected.jsonl` from the TypeScript SDK (`make generate-conformance`) -- this produces the correct expected output since the TS SDK handles round-tripping correctly
- [x] 1.4 Run the Go conformance test and confirm it FAILS (`cd test/conformance && go test -tags conformance -v -run multi-step-web-search ./anthropic/`) -- this is our failing test that drives the implementation

## 2. Fix orchestration layer (appendToolResults)

- [x] 2.1 Modify `appendToolResults` in `streamtext.go` to check `ProviderExecuted` on each `ToolResult` and route provider-executed results as `ToolResultContentPart` inline in the `AssistantMessage.Content` instead of into the `ToolMessage`
- [x] 2.2 Carry through `ProviderMetadata` from `ToolCall` to `ToolCallContentPart.ProviderOptions` and from `ToolResult` to `ToolResultContentPart.ProviderOptions` in `appendToolResults`
- [x] 2.3 Add unit tests for `appendToolResults` covering: provider-executed only, non-provider-executed only, mixed, and ProviderMetadata propagation

## 3. Fix Anthropic provider-executed tool call conversion

- [x] 3.1 Modify `convertAssistantContent` in `anthropic/convert_request.go` to check `ProviderExecuted` on `ToolCallContentPart` and emit `server_tool_use` blocks for provider-executed tool calls (dispatching on provider tool name for code_execution sub-tools and programmatic-tool-call type stripping)
- [x] 3.2 Add unit tests for provider-executed tool call -> `server_tool_use` conversion covering: web_search, code_execution, bash_code_execution sub-tool, text_editor_code_execution sub-tool, programmatic-tool-call stripping, tool_search variants, and unknown tool name warning

## 4. Fix Anthropic inline tool result conversion

- [x] 4.1 Add `case provider.ToolResultContentPart` to `convertAssistantContent` with dispatch logic for `mcp_tool_result` (via mcpToolUseIDs), `web_search_tool_result`, `web_fetch_tool_result`, `tool_search_tool_result`, and code execution result variants
- [x] 4.2 Implement code execution result dispatch within the new case: `code_execution_tool_result` (including encrypted variant), `bash_code_execution_tool_result`, `text_editor_code_execution_tool_result`, and error handling
- [x] 4.3 Add unit tests for inline `ToolResultContentPart` conversion covering all tool result types and the warning case for unrecognized tools

## 5. Integration and verification

- [x] 5.1 Run the conformance test -- multi-step round-trip works (second step executes), remaining mismatch at chunk 6 is pre-existing `encryptedContent`/`encrypted_content` JSON naming issue in web search result serialization (same failure exists in single-step `upstream/web-search-tool` test)
- [x] 5.2 Run existing tests (`make test`) and verify no regressions
