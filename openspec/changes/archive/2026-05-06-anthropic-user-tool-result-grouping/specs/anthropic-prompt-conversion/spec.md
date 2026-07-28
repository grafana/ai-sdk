## ADDED Requirements

### Requirement: Consecutive user and tool messages merge into one Anthropic user block

The Anthropic provider SHALL group consecutive `provider.Message` entries with `Role == provider.RoleUser` or `Role == provider.RoleTool` into a single Anthropic API user message before serialization. The merged user message's `content` array SHALL contain the concatenation of every part from every source message in the run, in source order. A `RoleTool` message in the middle of a sequence SHALL NOT open a new Anthropic message; it SHALL append into the current user block (or open one if none is active). This mirrors upstream `groupIntoBlocks` (`packages/anthropic/src/convert-to-anthropic-prompt.ts:1129`) exactly.

#### Scenario: Adjacent RoleUser then RoleTool merge into one user message
- **WHEN** the prompt is `[RoleUser([text("hi")]), RoleTool([tool_result(call_1, "out")])]`
- **THEN** the resulting Anthropic API request SHALL contain exactly one `BetaMessageParam` with `Role: "user"` whose content is `[OfText{Text: "hi"}, OfToolResult{ToolUseID: "call_1", ...}]` in that order

#### Scenario: Adjacent RoleAssistant then RoleTool then RoleUser produces one assistant message followed by one merged user message
- **WHEN** the prompt is `[RoleAssistant([tool_call(call_1)]), RoleTool([tool_result(call_1, "out")]), RoleUser([text("ok"))]]`
- **THEN** the resulting request SHALL contain exactly two `BetaMessageParam` entries: an assistant message carrying the tool_use, then a single user message whose content is `[OfToolResult{ToolUseID: "call_1", ...}, OfText{Text: "ok"}]` in that order

#### Scenario: Standalone RoleTool produces one user message
- **WHEN** the prompt is `[RoleTool([tool_result(call_1, "out")])]`
- **THEN** the resulting request SHALL contain exactly one `BetaMessageParam` with `Role: "user"` whose single content block is `OfToolResult{ToolUseID: "call_1", ...}`

#### Scenario: Three consecutive same-role tool messages merge into one user message
- **WHEN** the prompt is `[RoleTool([tool_result(c1, "r1")]), RoleTool([tool_result(c2, "r2")]), RoleTool([tool_result(c3, "r3")])]`
- **THEN** the resulting request SHALL contain exactly one `BetaMessageParam` with `Role: "user"` and three tool_result blocks in `[c1, c2, c3]` order

### Requirement: User-role messages accept tool_result content parts

The Anthropic provider SHALL convert `provider.ContentPart` entries of `Type == ContentPartTypeToolResult` to Anthropic API `tool_result` (or `mcp_tool_result`) blocks when they appear inside a `RoleUser` provider message. The resulting block SHALL be identical to what the same `ContentPart` produces when it appears inside a `RoleTool` provider message. Tool-result parts SHALL NOT be silently dropped from `RoleUser` messages.

#### Scenario: Mixed tool_result and text in one user message preserves both blocks in order
- **WHEN** a single `RoleUser` provider message has `Content == [tool_result(call_1, "out"), text("then text")]`
- **THEN** the resulting Anthropic user message's content SHALL be `[OfToolResult{ToolUseID: "call_1", ...}, OfText{Text: "then text"}]` in that order — neither block dropped, neither reordered

#### Scenario: Tool_result-only user message is identical to standalone tool message
- **WHEN** the prompt is `[RoleUser([tool_result(call_1, "out")])]` and separately `[RoleTool([tool_result(call_1, "out")])]`
- **THEN** the resulting Anthropic API request `Messages` SHALL be byte-identical between the two cases

#### Scenario: MCP tool_result inside a user message produces an mcp_tool_result block
- **WHEN** the prompt contains an assistant `mcp_tool_use` for `toolCallID = mcp-1` followed by a `RoleUser` with content `[tool_result(mcp-1, ...)]`
- **THEN** the resulting Anthropic user message SHALL contain `OfMCPToolResult{ToolUseID: "mcp-1", ...}` (not a plain `OfToolResult`)

### Requirement: Tool approval responses inside user-role messages are skipped silently

The Anthropic provider SHALL skip `provider.ContentPart` entries of `Type == ContentPartTypeToolApprovalResponse` when they appear inside a `RoleUser` provider message. No Anthropic content block SHALL be emitted for them. No warning SHALL be added (mirrors upstream's `if (part.type === 'tool-approval-response') { continue; }` skip in the user-block handler). Approval responses inside `RoleTool` messages keep their existing warning-and-drop behavior; this requirement only governs the `RoleUser` case.

#### Scenario: Approval response inside a user message with text yields only the text block
- **WHEN** a `RoleUser` provider message has `Content == [tool_approval_response("a1", true), text("hello")]`
- **THEN** the resulting Anthropic user message's content SHALL be `[OfText{Text: "hello"}]` — the approval response SHALL produce no block and SHALL NOT add a warning to the request

### Requirement: Cache-control cascade is keyed off source provider message, not merged Anthropic block

When `provider.Message` entries are merged into a single Anthropic user (or assistant) block, the Anthropic provider SHALL apply each source message's `ProviderOptions["anthropic"].cache_control` cascade to the *last part of that source message's `Content` slice* — not to the last part of the merged Anthropic block. Each source message participates in the breakpoint budget independently. This preserves the upstream cascade semantics (`packages/anthropic/src/convert-to-anthropic-prompt.ts:130-145, 326-338`).

#### Scenario: Per-source-message message-level cascade survives the merge
- **WHEN** the prompt is `[RoleTool([tool_result(call_1, "r1")] with msgOpts cache_control=ephemeral), RoleUser([text("hello")] with no opts)]`
- **THEN** the resulting Anthropic user message's *first* content block (the tool_result) SHALL carry `cache_control: ephemeral`, and the *second* block (the text) SHALL NOT carry `cache_control`

#### Scenario: Two source messages with cache_control each produce their own breakpoint
- **WHEN** the prompt is `[RoleTool([tool_result(call_1, "r1")] with msgOpts cache_control=ephemeral), RoleUser([text("hello")] with msgOpts cache_control=ephemeral)]`
- **THEN** both content blocks in the merged Anthropic user message SHALL carry `cache_control: ephemeral`, and the cache-breakpoint count SHALL be 2

#### Scenario: Part-level cache_control on a non-last part survives unchanged after merge
- **WHEN** a `RoleUser` provider message has `Content == [text("a") with partOpts cache_control=ephemeral, tool_result(call_1, "r1") with no opts]` and is followed by a `RoleTool` provider message with no opts
- **THEN** the merged Anthropic user message's first text block SHALL carry `cache_control: ephemeral`; the tool_result block SHALL NOT carry one (it was non-last in its source message and the source had no message-level cascade)

### Requirement: Consecutive assistant messages merge into one Anthropic assistant block

The Anthropic provider SHALL group consecutive `provider.Message` entries with `Role == provider.RoleAssistant` into a single Anthropic API assistant message. The merged assistant message's `content` array SHALL contain the concatenation of every part from every source message in the run, in source order. The cache-control cascade and ProviderExecuted-vs-MCP-vs-standard tool-call dispatch in `convertAssistantContent` SHALL be applied per source message, not per merged block.

#### Scenario: Two consecutive assistant messages merge into one
- **WHEN** the prompt is `[RoleAssistant([text("hi")]), RoleAssistant([tool_call("c1", "search", {})])]`
- **THEN** the resulting request SHALL contain exactly one `BetaMessageParam` with `Role: "assistant"` whose content is `[OfText{Text: "hi"}, OfToolUse{ID: "c1", Name: "search", ...}]` in that order

### Requirement: System messages continue to flow into p.System

`provider.Message` entries with `Role == provider.RoleSystem` SHALL continue to append `BetaTextBlockParam` entries onto the request's top-level `System` array (not into `Messages`). The grouping pre-pass SHALL NOT change the per-message cache-control resolution for system messages: each source system message's `ProviderOptions["anthropic"].cache_control` continues to apply to its own emitted `BetaTextBlockParam`.

#### Scenario: System message produces a system block, not a user message
- **WHEN** the prompt is `[RoleSystem("you are helpful"), RoleUser([text("hi")])]`
- **THEN** the resulting request SHALL have one entry in `System` with `Text == "you are helpful"` and exactly one `BetaMessageParam` with `Role: "user"`

#### Scenario: Two consecutive system messages each emit a system block
- **WHEN** the prompt is `[RoleSystem("a"), RoleSystem("b"), RoleUser([text("hi")])]`
- **THEN** the resulting request SHALL have two entries in `System` (`Text == "a"` then `Text == "b"`), and exactly one `BetaMessageParam` with `Role: "user"`

### Requirement: Single shared helper builds Anthropic tool_result blocks

The Anthropic provider SHALL share a single internal helper that converts a `provider.ContentPart` of type `ContentPartTypeToolResult` into the corresponding Anthropic API content block (`OfToolResult` for standard tools, `OfMCPToolResult` when the tool-call ID was emitted by a prior MCP tool-use). Both `convertUserContent` (for tool-result parts inside `RoleUser` messages) and `convertToolContent` (for tool-result parts inside `RoleTool` messages) SHALL call this helper. The helper SHALL apply the same MCP-vs-standard branching, the same `serializeToolOutput` path, and the same `serializeMCPToolResultContent` path that today's `convertToolContent` uses.

#### Scenario: Same tool_result produces same block from RoleUser and RoleTool inputs
- **WHEN** an identical `tool_result(call_1, output)` part is converted via `convertUserContent` and via `convertToolContent`
- **THEN** the resulting Anthropic API content block SHALL be byte-identical (same `ToolUseID`, same `Content`, same `IsError`, same `CacheControl`)

#### Scenario: MCP tool_result detection works regardless of source role
- **WHEN** the prompt contains a prior assistant `mcp_tool_use(call_1)` and a tool_result for `call_1` appears inside either a `RoleUser` or a `RoleTool` provider message
- **THEN** in both cases the resulting Anthropic block SHALL be `OfMCPToolResult` (not `OfToolResult`)

### Requirement: Wire-format and public API are unchanged

This requirement closes the contract: the change is implementation-internal to `anthropic/`. The `provider.LanguageModel` interface, the `BuildParams` signature, the SSE chunk types emitted by the orchestration layer, and the request body shape POSTed to Anthropic's Messages API SHALL all be byte-identical to the pre-change behavior for any prompt that does not exercise the user/tool grouping or tool-result-in-user-message paths.

#### Scenario: Non-grouping prompts produce identical wire requests
- **WHEN** a prompt that contains no consecutive user/tool messages and no tool-result parts inside user messages is converted
- **THEN** the resulting Anthropic API request body SHALL be byte-identical to the pre-change output for the same input
