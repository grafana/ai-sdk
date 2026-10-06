## Why

Two related parity gaps in the Go anthropic adapter's prompt conversion prevent multi-turn prompts from round-tripping correctly to Anthropic's Messages API:

1. **`convertUserContent` (`anthropic/convert_request.go:287`) silently drops `tool_result` blocks.** The `provider.ContentPart` switch only handles `Text` and `File`; any `ContentPartTypeToolResult` part inside a `RoleUser` message is omitted from the request. The dedicated `convertToolContent` (line 1121) is reachable only when the message has `Role: provider.RoleTool`. A user turn that mixes `[tool_result, text]` (e.g. Lodestone's `injectDynamicContext` appending text to a returned tool-result message) cannot survive conversion.

2. **No `groupIntoBlocks` equivalent.** The per-message switch (`convert_request.go:107-134`) emits one Anthropic message per `provider.Message`, so consecutive `RoleTool` + `RoleUser` (or `RoleUser` + `RoleTool`) provider messages produce two adjacent Anthropic user messages instead of one merged user message containing both `[tool_result, ..., text, ...]`. Upstream `groupIntoBlocks` (`packages/anthropic/src/convert-to-anthropic-prompt.ts:1129`) merges them into a single user block before serialization.

Together, the symptom downstream is `400 Bad Request: messages.N: tool_use ids were found without tool_result blocks immediately after`, observed in `grafana/grafana-assistant-app` Lodestone after the ai-sdk migration. This change closes #173 and aligns the converter with upstream behavior. Suggested PR title: `fix(anthropic): handle tool_result in user messages, merge consecutive user/tool blocks`.

## What Changes

- Extend `convertUserContent` in `anthropic/convert_request.go` to handle `ContentPartTypeToolResult` blocks (both standard and MCP-tagged tool-results) so a `RoleUser` message containing a mix of text/file/tool-result parts converts losslessly. Refactor the inner block-building shared with `convertToolContent` into a reusable helper so the two call sites stay in sync.
- Extend `convertUserContent` to drop `ContentPartTypeToolApprovalResponse` parts with no error (matching upstream's `if (part.type === 'tool-approval-response') { continue; }` skip in the combined user-block handler), so a future producer-side change that emits approval responses on `RoleUser` does not regress.
- Add a `groupIntoBlocks` pre-pass over `[]provider.Message` that merges consecutive `RoleUser` and `RoleTool` provider messages into a single Anthropic user block. The pre-pass also collapses adjacent `RoleAssistant` messages into a single Anthropic assistant block and adjacent `RoleSystem` messages into a single system block, matching upstream's `groupIntoBlocks` shape exactly. Per-block conversion replaces the existing per-message switch in `buildParams`.
- Preserve the existing cache-control cascade semantics: per-part `ProviderOptions` win; otherwise the *originating* `provider.Message`'s `ProviderOptions` apply only to the last part of that message inside the merged block (mirrors upstream's `validator.getCacheControl(message.providerOptions, ...)` keyed off the per-source-message last-part flag, not the merged-block last-part flag).
- Preserve the existing assistant-block "trim trailing whitespace on the last text part of the last block" behavior so multi-message assistant blocks still produce a clean prefill.
- Add unit tests covering: tool_result inside a user message, consecutive `RoleUser` + `RoleTool` grouping, three-way `RoleAssistant(tool_call) -> RoleTool(tool_result) -> RoleUser(text)` grouping, ordering preservation across the merge, and per-message cache-control cascade preservation across the merge.

## Capabilities

### New Capabilities

- `anthropic-prompt-conversion`: Provider-side conversion semantics from the V4 `provider.Message` / `provider.ContentPart` shape to Anthropic's Messages API request shape. Captures the rules that govern message grouping (consecutive `user`/`tool` merge into one Anthropic user block, consecutive `assistant` merge into one Anthropic assistant block, consecutive `system` merge into the prompt-level `system` array), per-role part dispatch (which `ContentPartType` is valid in which role), and the per-source-message cache-control cascade across merged blocks. This capability codifies the upstream `convertToAnthropicPrompt` + `groupIntoBlocks` contract for the Go port.

### Modified Capabilities

_None._ The existing `anthropic-prompt-caching` cache-control cascade behavior is preserved exactly; this change only widens the set of `ContentPart` types that survive `convertUserContent` and changes how multiple `provider.Message`s are grouped before per-block conversion runs.

## Impact

- **`anthropic/convert_request.go`**: `convertUserContent` extended to handle `ContentPartTypeToolResult` and skip `ContentPartTypeToolApprovalResponse`. New unexported `groupIntoBlocks` helper plus a per-block emission loop replaces the per-message switch in `buildParams`. New shared block-builder helper (e.g. `appendToolResultBlock`) extracted from `convertToolContent`'s `OfToolResult` / `OfMCPToolResult` branches and called from both `convertUserContent` (when iterating over user-role tool-result parts) and `convertToolContent` (when iterating over tool-role tool-result parts).
- **`anthropic/convert_request_test.go`**: new tests for the four scenarios above; existing tests stay green with no behavior change.
- **No public API change.** `buildParams` keeps its signature; the change is internal to the anthropic provider's prompt-conversion path.
- **No wire-format change.** SSE chunk types and `@ai-sdk/react` compatibility are unaffected — this fix is upstream of the orchestration layer.
- **Closes #173.** Removes the need for the workaround in `grafana/grafana-assistant-app` Lodestone (`internal/agentic/lodestone/aisdk/messages.go:MessagesFromAnthropic` splits mixed user messages at the engine boundary).
- **Related (out of scope here)**: #171 (`appendToolResults` reasoning gap), #172 (public `ToResponseMessages` helper). Both are independent and tracked separately.
