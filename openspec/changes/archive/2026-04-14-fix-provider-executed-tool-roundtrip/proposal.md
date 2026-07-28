## Why

Provider-executed tool results (from server tools like `web_search`, `code_execution`, `tool_search`) are silently lost during multi-turn conversations. The first turn works correctly -- server tool calls and results stream and render fine. But on the next turn, when the full conversation history is sent back to the API, two independent bugs produce a malformed request that breaks the conversation.

## What Changes

- Fix `appendToolResults` in the orchestration layer (`streamtext.go`) to route provider-executed tool results inline in the `AssistantMessage.Content` instead of into a separate `ToolMessage`, matching upstream `toResponseMessages` behavior.
- Fix `convertAssistantContent` in the Anthropic provider (`anthropic/convert_request.go`) to handle `provider.ToolResultContentPart` entries that appear inline in assistant messages, converting them to the appropriate Anthropic API block format (e.g., `server_tool_result`, `web_search_tool_result`, `code_execution_tool_result`, etc.).
- Fix `convertAssistantContent` to emit `server_tool_use` blocks (instead of regular `tool_use`) for `ToolCallContentPart` entries with `ProviderExecuted: true`, matching upstream behavior.

## Capabilities

### New Capabilities

- `provider-executed-tool-roundtrip`: Correct round-tripping of provider-executed tool calls and results through multi-turn conversations, covering both the orchestration layer message building and the Anthropic provider request conversion.

### Modified Capabilities


## Impact

- `streamtext.go`: `appendToolResults` function changes how multi-step messages are constructed for provider-executed tools.
- `anthropic/convert_request.go`: `convertAssistantContent` function adds handling for `ToolResultContentPart` and `ProviderExecuted` flag on `ToolCallContentPart`.
- Affects all multi-turn conversations that use Anthropic server-side tools (`web_search`, `code_execution`, `tool_search`, computer use, etc.).
- No API surface changes -- this is a behavioral fix in existing code paths.
