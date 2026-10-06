## 1. Extract shared tool-result block-builder helper

- [x] 1.1 In `anthropic/convert_request.go`, extract the inner block-building logic from `convertToolContent` into a new unexported helper `appendToolResultBlock(blocks []anthropic.BetaContentBlockParamUnion, p provider.ContentPart, cc anthropic.BetaCacheControlEphemeralParam, mcpToolUseIDs map[string]bool, warnings *[]provider.Warning) []anthropic.BetaContentBlockParamUnion`. The helper SHALL contain exactly the MCP-vs-standard branching, `serializeMCPToolResultContent` call, and `serializeToolOutput` call that exist today.
- [x] 1.2 Refactor `convertToolContent` to call `appendToolResultBlock` for `ContentPartTypeToolResult` parts. Keep the per-part cache-control resolution (`v.resolveCacheControl(p.ProviderOptions, msgOpts, isLast, true)`) in the caller; the helper takes the resolved cache-control as a parameter. Keep the existing `ContentPartTypeToolApprovalResponse` warning-and-skip behavior in `convertToolContent` (do not change tool-role approval handling).
- [x] 1.3 Verify by running `go test -count=1 ./...` from `anthropic/` that all existing tests pass with no behavior change. (This task is a pure refactor; no test changes expected here.)

## 2. Extend convertUserContent for tool-result and tool-approval-response parts

- [x] 2.1 Update the signature of `convertUserContent` to accept `mcpToolUseIDs map[string]bool` and `warnings *[]provider.Warning`. (`betas *[]anthropic.AnthropicBeta` and `msgOpts` are already there.) Wire the new arguments at the single existing call site in `buildParams`.
- [x] 2.2 Add a `case provider.ContentPartTypeToolResult:` branch to `convertUserContent`'s switch. Resolve cache-control via `v.resolveCacheControl(p.ProviderOptions, msgOpts, isLast, true)` (same as the text/file branches), then call `appendToolResultBlock` with the resolved cache-control.
- [x] 2.3 Add a `case provider.ContentPartTypeToolApprovalResponse:` branch to `convertUserContent`'s switch that does nothing (silent skip), matching upstream's `if (part.type === 'tool-approval-response') { continue; }` in the user-block handler. Do NOT emit a warning. (The existing `convertToolContent` keeps its own approval-response warning behavior; only the user-role path is silent.)
- [x] 2.4 Confirm `convertUserContent` still produces a `nil` return for an empty input (current behavior); no change to the empty-input path.

## 3. Port groupIntoBlocks pre-pass

- [x] 3.1 Add an unexported `promptBlockKind` typed `int` enum to `anthropic/convert_request.go` with constants `promptBlockKindSystem`, `promptBlockKindUser`, `promptBlockKindAssistant` (idiomatic Go enum, package-private).
- [x] 3.2 Add an unexported `promptBlock` struct: `{kind promptBlockKind; messages []provider.Message}`.
- [x] 3.3 Add an unexported `groupIntoBlocks(prompt []provider.Message) []promptBlock` helper that mirrors upstream `groupIntoBlocks` exactly:
    - `RoleSystem` → append to current `promptBlockKindSystem` block, else open a new one.
    - `RoleAssistant` → append to current `promptBlockKindAssistant` block, else open a new one.
    - `RoleUser` → append to current `promptBlockKindUser` block, else open a new one.
    - `RoleTool` → append to current `promptBlockKindUser` block, else open a new one.
    - Any unknown role → open a new block of that role's natural kind (system/user/assistant); the existing per-role switch in `buildParams` already exhaustively handles the four valid roles, so an unknown role falls through silently — preserve that current behavior.

## 4. Wire the pre-pass into buildParams

- [x] 4.1 In `buildParams`, replace `for _, msg := range opts.Prompt { switch msg.Role ... }` with `for _, block := range groupIntoBlocks(opts.Prompt) { switch block.kind ... }`.
- [x] 4.2 For `promptBlockKindSystem`: iterate `block.messages` and append a `BetaTextBlockParam` to `p.System` per message, exactly as the current `case provider.RoleSystem:` arm does today. Cache-control resolution stays per source message.
- [x] 4.3 For `promptBlockKindUser`: build a single `[]anthropic.BetaContentBlockParamUnion` by iterating `block.messages` in order and, for each source message, dispatching on `msg.Role`:
    - `RoleUser` → call `convertUserContent(v, msg.Content, msg.ProviderOptions, &p.Betas, mcpToolUseIDs, &warnings)` and append the returned blocks.
    - `RoleTool` → call `convertToolContent(v, msg.Content, msg.ProviderOptions, mcpToolUseIDs, &warnings)` and append the returned blocks.
    Append exactly one `BetaMessageParam{Role: "user", Content: <merged>}` to `p.Messages`.
- [x] 4.4 For `promptBlockKindAssistant`: build a single `[]anthropic.BetaContentBlockParamUnion` by iterating `block.messages` and calling `convertAssistantContent(v, mapping, msg.Content, msg.ProviderOptions, mcpToolUseIDs, &warnings)` per message; append exactly one `BetaMessageParam{Role: "assistant", Content: <merged>}` to `p.Messages`.
- [x] 4.5 Verify that all four `case` arms in the new `switch block.kind` continue to use the *single* shared `cacheControlValidator v` and the *single* shared `mcpToolUseIDs` map, so cache-breakpoint counting and MCP-tool-use ID tracking work correctly across the merged blocks (mirrors upstream's single per-request `validator` and `mcpToolUseIds` Set).

## 5. Tests in anthropic/convert_request_test.go

All tests use the existing pattern (`buildParams` + `require`/`assert`); place new tests next to related existing tests (e.g. near `TestBuildParams_ToolMessage`).

- [x] 5.1 `TestBuildParams_UserMessageWithToolResult`: a single `RoleUser` message containing `[ToolResult(call_1, "out"), Text("hello")]`. Assert exactly one `BetaMessageParam` with `Role: "user"` and content order `[OfToolResult, OfText]` carrying the right `ToolUseID` and text.
- [x] 5.2 `TestBuildParams_ConsecutiveUserAndToolMessagesMerge`: prompt `[RoleUser([Text("hi")]), RoleTool([ToolResult(call_1, "out")])]`. Assert exactly one `BetaMessageParam` (not two) with content `[OfText, OfToolResult]`.
- [x] 5.3 `TestBuildParams_ConsecutiveToolAndUserMessagesMerge`: prompt `[RoleTool([ToolResult(call_1, "out")]), RoleUser([Text("ok")])]`. Assert exactly one `BetaMessageParam` with content `[OfToolResult, OfText]`.
- [x] 5.4 `TestBuildParams_AssistantToolUserGrouping`: prompt `[RoleAssistant([ToolCall(call_1, "search", {})]), RoleTool([ToolResult(call_1, "out")]), RoleUser([Text("ok")])]`. Assert exactly two `BetaMessageParam` entries — assistant carrying the tool_use, then user carrying `[OfToolResult, OfText]` in that order.
- [x] 5.5 `TestBuildParams_StandaloneToolMessage`: prompt `[RoleTool([ToolResult(call_1, "out")])]`. Assert exactly one user `BetaMessageParam` with one `OfToolResult` block. (Regression guard: standalone tool messages must keep working.)
- [x] 5.6 `TestBuildParams_ConsecutiveAssistantMessagesMerge`: prompt `[RoleAssistant([Text("hi")]), RoleAssistant([ToolCall("c1", "search", {})])]`. Assert exactly one assistant `BetaMessageParam` with content `[OfText, OfToolUse]`.
- [x] 5.7 `TestBuildParams_ThreeConsecutiveToolMessagesMerge`: prompt of three back-to-back `RoleTool` messages, each with a different `ToolResult`. Assert exactly one user `BetaMessageParam` with three `OfToolResult` blocks in source order.
- [x] 5.8 `TestBuildParams_ApprovalResponseInUserMessageSilentlySkipped`: prompt `[RoleUser([ToolApprovalResponse("a1", &approved=true, ""), Text("hello")])]`. Assert exactly one user `BetaMessageParam` with content `[OfText{Text:"hello"}]` (no approval-response block emitted) and no warnings about approval responses.
- [x] 5.9 `TestBuildParams_CacheControlCascadePreservedAcrossMerge`: prompt `[RoleTool([ToolResult(c1, "r1")] with msgOpts={cacheControl:ephemeral}), RoleUser([Text("hello")] with msgOpts={cacheControl:ephemeral})]`. Assert the merged user message has two content blocks, both carrying `cache_control.Type == "ephemeral"`, and (optionally — if a hook is exposed) the validator's breakpoint count is 2.
- [x] 5.10 `TestBuildParams_CacheControlSourceMessageScoped`: prompt `[RoleTool([ToolResult(c1, "r1")] with msgOpts={cacheControl:ephemeral}), RoleUser([Text("hello")] with NO opts)]`. Assert the *first* merged block (the tool_result) carries cache_control and the *second* (the text) does not — i.e. the cascade is keyed off the source-message last-part, not the merged-block last-part.
- [x] 5.11 `TestBuildParams_MCPToolResultInUserMessage`: prompt `[RoleAssistant([ToolCall(mcp-1, "echo", {}, providerOpts={anthropic:{type:"mcp-tool-use",serverName:"s"}})]), RoleUser([ToolResult(mcp-1, ...)])]`. Assert the resulting user message's content block is `OfMCPToolResult` (not `OfToolResult`) with `ToolUseID == "mcp-1"`.
- [x] 5.12 `TestBuildParams_NonGroupingPromptUnchanged`: a prompt that does not exercise any merge or tool-result-in-user path (e.g. `[RoleSystem, RoleUser([Text]), RoleAssistant([Text])]`). Assert the resulting `Messages` and `System` arrays are byte-equivalent to the pre-change output (regression guard).
- [x] 5.13 `TestGroupIntoBlocks_GroupingRules`: table-driven unit test on `groupIntoBlocks` directly covering empty input, system+user, user/tool/user merge into one block, assistant/user/assistant alternation, and consecutive system messages merging into one block. (Added on top of section-5 plan; gives direct white-box coverage of the pre-pass.)

## 6. Lint, build, and verification

- [x] 6.1 From `anthropic/`: `go build ./...` — succeeds.
- [x] 6.2 From `anthropic/`: `go test -count=1 ./...` — all tests pass (existing + new).
- [x] 6.3 From `anthropic/`: `go vet ./...` — clean.
- [x] 6.4 From repo root: `make lint` — clean for `anthropic/` and `providers/grafana/` (`0 issues.`). Root module fails `make lint` and `make vet` due to an in-flight refactor of `streamtext.go` / `streamtext_test.go` in a sibling subagent's worktree (`appendToolResults` arity changed for issues #171/#172); those failures are unrelated to this change. `make test-anthropic` and `make test-grafana` both pass.
- [x] 6.5 From repo root: `make test-anthropic` and `make test-grafana` — pass. Root-module `make test` is currently red (sibling subagent's WIP); not caused by this change. Per `git status`, the only files modified by this change are `anthropic/convert_request.go` and `anthropic/convert_request_test.go` plus the new openspec change folder.
- [x] 6.6 Reviewed the diff in `anthropic/convert_request.go`: changes are confined to `convertUserContent` (signature + new switch arms), `convertToolContent` (now delegates to helper), `buildParams` (new `groupIntoBlocks`-driven loop), and the new `appendToolResultBlock`, `promptBlockKind`, `promptBlock`, and `groupIntoBlocks` declarations. No other functions touched. Blast radius confined to the anthropic submodule.
