## 1. Tool Name Mapping Expansion

- [x] 1.1 Add 12 new entries to `providerToolNames` map in `anthropic/tool_name_mapping.go`: code_execution x3, computer x3, text_editor x4 (with correct wire names `str_replace_editor` vs `str_replace_based_edit_tool`), bash x2
- [x] 1.2 Add unit tests for the new mapping entries

## 2. Beta Header Threading

- [x] 2.1 Change `convertProviderTool` signature to return `(BetaToolUnionParam, []string, *provider.Warning)` and update existing cases (web_search, tool_search) to return nil/empty betas
- [x] 2.2 Update `convertTools` to merge betas from `convertProviderTool` into the beta set
- [x] 2.3 Add unit tests verifying betas flow from provider tools through to the request params

## 3. Tool Definitions (Request Building)

- [x] 3.1 Add `convertProviderTool` cases for `code_execution_20250522`, `code_execution_20250825`, `code_execution_20260120` with correct types, names, and betas
- [x] 3.2 Add `convertProviderTool` cases for `computer_20241022`, `computer_20250124`, `computer_20251124` with display dimension arg extraction and betas
- [x] 3.3 Add `convertProviderTool` cases for `text_editor_20241022`, `text_editor_20250124`, `text_editor_20250429`, `text_editor_20250728` with correct names and `maxCharacters` extraction for 20250728
- [x] 3.4 Add `convertProviderTool` cases for `bash_20241022`, `bash_20250124` with betas
- [x] 3.5 Add unit tests for each tool ID verifying type, name, args, and beta output

## 4. Dynamic Flag

- [x] 4.1 Implement `hasWebTool20260209WithoutCodeExecution` helper that checks prepared tools for web_fetch/web_search 20260209 types without a `code_execution` named tool
- [x] 4.2 Compute `markCodeExecutionDynamic` in both `DoStream` and `DoGenerate` after tool preparation, thread to stream adapter and `convertResponse`
- [x] 4.3 Add unit tests for the helper function covering all flag conditions

## 5. Streaming Response: Input Rewriting

- [x] 5.1 Add `firstDelta bool` and `providerToolName string` fields to `blockState` struct
- [x] 5.2 In `content_block_start` for `server_tool_use`: store original wire name as `providerToolName`, map `bash_code_execution`/`text_editor_code_execution` through `code_execution` for the custom tool name, set `firstDelta = true` when no pre-populated input
- [x] 5.3 In `input_json_delta` handler: for blocks where `providerToolName` is `bash_code_execution` or `text_editor_code_execution` and `firstDelta` is true and delta is non-empty, rewrite first delta to inject `{"type": "<providerToolName>",`
- [x] 5.4 In `content_block_stop` for `server_tool_use`: when `providerToolName` is `code_execution`, check accumulated input for `code` field without `type` field, inject `"type": "programmatic-tool-call"` if applicable
- [x] 5.5 Add unit tests for delta rewriting (first delta, empty delta skipping, subsequent deltas) and programmatic tool call injection

## 6. Non-Streaming Response: Input Rewriting

- [x] 6.1 In `convertResponse` for `server_tool_use`: when name is `bash_code_execution` or `text_editor_code_execution`, wrap input as `{"type": "<name>", ...input}` and map tool name through `code_execution`
- [x] 6.2 In `convertResponse` for `server_tool_use`: when name is `code_execution` and input has `code` but no `type`, inject `"type": "programmatic-tool-call"`
- [x] 6.3 Apply `markCodeExecutionDynamic` flag: set `Dynamic: true` on `code_execution` tool call parts when flag is true
- [x] 6.4 Add unit tests for non-streaming input wrapping, programmatic injection, and dynamic flag

## 7. Result Block Handling

- [x] 7.1 Add streaming handler for `code_execution_tool_result` blocks (handle all 3 subtypes: result, encrypted, error), emit `PartToolResult` with tool name from `toCustomToolName("code_execution")`
- [x] 7.2 Add streaming handlers for `bash_code_execution_tool_result` and `text_editor_code_execution_tool_result` blocks with pass-through content
- [x] 7.3 Add non-streaming handler for `code_execution_tool_result` blocks in `convertResponse`
- [x] 7.4 Add non-streaming handlers for `bash_code_execution_tool_result` and `text_editor_code_execution_tool_result` in `convertResponse`
- [x] 7.5 Add unit tests for all result block types in both streaming and non-streaming paths

## 8. Pre-populated Input and Content

- [x] 8.1 In `content_block_start` for `tool_use` and `server_tool_use`: check for non-empty `input` object, pre-serialize to `accumulatedInput`, set `firstDelta` based on presence
- [x] 8.2 In `message_start` handler: iterate pre-populated `content` array, emit full tool-input lifecycle (start -> delta -> end -> call) for each `tool_use` block with caller metadata
- [x] 8.3 Add unit tests for pre-populated input on content_block_start and pre-populated content on message_start

## 9. Integration Verification

- [x] 9.1 Run full test suite (`make test`) and fix any regressions
- [x] 9.2 Verify existing conformance tests still pass
- [x] 9.3 Run `make check` (fmt + vet + test) to ensure code quality
