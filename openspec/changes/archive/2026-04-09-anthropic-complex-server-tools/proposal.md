## Why

The Anthropic provider currently supports only web_search and tool_search as server tools. The upstream Vercel AI SDK supports 12 additional provider-defined tools (code execution, computer use, text editor, bash) across multiple versions, each with version-specific args, beta headers, input rewriting, and specialized result block handling. Adding these is required for feature parity and to unblock users relying on these capabilities.

## What Changes

- Add request-side support for 12 new provider-defined tool IDs: code_execution (3 versions), computer (3 versions), text_editor (4 versions), bash (2 versions)
- Change `convertProviderTool` to return beta headers alongside tool params, and thread them into API requests
- Add response handling for 3 new result block types: `code_execution_tool_result`, `bash_code_execution_tool_result`, `text_editor_code_execution_tool_result`
- Implement input rewriting for code_execution_20250825 (streaming delta rewriting and non-streaming input wrapping) and programmatic-tool-call type injection
- Add dynamic flag computation via `hasWebTool20260209WithoutCodeExecution` helper
- Handle pre-populated input on `tool_use` content_block_start (deferred tool calls from code_execution)
- Handle pre-populated content in `message_start` events containing `tool_use` blocks

## Capabilities

### New Capabilities
- `complex-server-tools`: Request building, beta header threading, response result blocks, input rewriting rules, and dynamic flag for code_execution, computer, text_editor, and bash provider-defined tools

### Modified Capabilities
- `server-tools`: Beta header return mechanism -- `convertProviderTool` must return betas alongside tool params so provider-defined tools can contribute required beta headers to the API request
- `tool-name-mapping`: Expand the static provider tool names table with entries for all 12 new tool IDs mapping to their Anthropic API wire names

## Impact

- **anthropic/convert_request.go**: `convertProviderTool` signature change (adds beta return), 12 new switch cases with version-specific arg extraction and validation
- **anthropic/convert_stream.go**: New result block handlers, delta rewriting state on `blockState`, `hasWebTool20260209WithoutCodeExecution` helper, pre-populated input/content handling
- **anthropic/convert_response.go**: New result block handlers, input wrapping for code_execution_20250825, programmatic tool call injection, dynamic flag
- **anthropic/tool_name_mapping.go**: 12+ new entries in `providerToolNames` map
- **anthropic/convert_tools.go** or equivalent: `convertTools` must propagate betas from `convertProviderTool` into the beta set
- **Dependencies**: Requires tool name mapping (#9) for name resolution on result blocks; builds on server tools foundation (#8)
