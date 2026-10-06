## Context

The Go anthropic adapter converts a `provider.CallOptions.Prompt` (`[]provider.Message`) into Anthropic's `BetaMessageNewParams` shape. The current converter (`anthropic/convert_request.go:107-134`) walks `Prompt` once and emits one Anthropic message per `provider.Message`:

```
provider.Message{Role: RoleSystem}    → append BetaTextBlockParam to p.System
provider.Message{Role: RoleUser}      → convertUserContent  → BetaMessageParam{Role: "user", ...}
provider.Message{Role: RoleAssistant} → convertAssistantContent → BetaMessageParam{Role: "assistant", ...}
provider.Message{Role: RoleTool}      → convertToolContent  → BetaMessageParam{Role: "user", ...}
```

This shape diverges from upstream (`packages/anthropic/src/convert-to-anthropic-prompt.ts`) in two ways:

1. The upstream converter pre-groups `LanguageModelV4Prompt` into typed blocks (`SystemBlock`, `UserBlock`, `AssistantBlock`) via `groupIntoBlocks`. A `UserBlock` collects every consecutive `user`/`tool` provider message and emits one Anthropic user message containing the concatenation of all their parts. The Go port emits one Anthropic message per provider message, so two consecutive `RoleTool` + `RoleUser` provider messages become two adjacent `role: "user"` messages on the wire — Anthropic's API rejects this when a `tool_use` was issued in the immediately-prior assistant message because the `tool_result` block ends up in the *second* user message rather than the *first*.

2. The upstream user-block handler accepts `tool_result` parts inside a `user` provider message and dispatches them through the same content-building branch as the `tool` provider message (a single switch on `part.type`). The Go port's `convertUserContent` only handles `text` and `file` parts, so any caller that constructs `RoleUser` with a tool-result part — including downstream code that appends text to a tool-result-bearing message before sending — silently loses the tool-result block.

Per `.cursor/rules/upstream-parity.mdc`, Go is a port of upstream behavior. Adapting to Go idioms is encouraged; diverging from upstream semantics is not. This design closes both gaps with a minimal, well-scoped refactor inside `anthropic/convert_request.go`.

## Goals / Non-Goals

**Goals:**

- Behavioral parity with upstream `convertToAnthropicPrompt` + `groupIntoBlocks` for the four roles (`system`, `user`, `assistant`, `tool`) — multi-message prompts that mix tool-result and text in the same logical user turn must produce the same Anthropic API request shape they do upstream.
- A single source of truth for "how to build an Anthropic `tool_result` content block from a `provider.ContentPart`": the helper used inside `convertToolContent` is reused inside `convertUserContent`'s tool-result branch.
- Cache-control cascade behavior preserved: per-part `ProviderOptions` win; when a part is the *last part of its source provider message*, the source message's `ProviderOptions` cascade applies. The merge into a single Anthropic block must not silently discard or duplicate cache-control hits.
- Tests cover every scenario listed in `proposal.md` plus an explicit ordering invariant (a `[tool_result, text]` user message produces blocks in `[tool_result, text]` order on the wire).
- No public API surface change; no wire-format change visible to `@ai-sdk/react`.

**Non-Goals:**

- Validating that `tool_result` parts always have a matching `tool_use` in the prior assistant block. Mirrors upstream: validation is producer-side, not converter-side. The converter trusts its input.
- Supporting `ContentPartTypeToolApprovalResponse` semantics. Upstream skips them in the user-block handler with `continue`; the Go port follows suit. (`convertToolContent` already emits a warning for these in the `tool` role; that path is unchanged.)
- Refactoring `convertAssistantContent` or `convertSystemContent`. Their per-part dispatch is already correct; the only change for assistant/system blocks is that the *outer* per-block loop now hands them lists of consecutive same-role messages instead of single messages.
- Changing the cache-control validator (`anthropic/cache_control.go`). Its `getCacheControl` / `resolveCacheControl` semantics are reused as-is.
- Modifying any file outside `anthropic/`. Root-module orchestration (`streamtext.go`, `convert.go`, etc.) is owned by a sibling change and must not be touched.

## Decisions

### D1. Port `groupIntoBlocks` as a Go pre-pass over `[]provider.Message`

Introduce an unexported `groupIntoBlocks(prompt []provider.Message) []promptBlock` helper in `anthropic/convert_request.go`. `promptBlock` is a small unexported struct discriminated by `kind` (`promptBlockKindSystem` / `promptBlockKindUser` / `promptBlockKindAssistant`) and carrying the slice of source `provider.Message` values that belong to it.

```go
type promptBlockKind int

const (
    promptBlockKindSystem promptBlockKind = iota
    promptBlockKindUser
    promptBlockKindAssistant
)

type promptBlock struct {
    kind     promptBlockKind
    messages []provider.Message
}
```

Grouping rules (mirror upstream `groupIntoBlocks` exactly):

| Source role     | Appends to current block of kind | Otherwise opens                |
| --------------- | -------------------------------- | ------------------------------ |
| `RoleSystem`    | `promptBlockKindSystem`          | new `promptBlockKindSystem`    |
| `RoleUser`      | `promptBlockKindUser`            | new `promptBlockKindUser`      |
| `RoleTool`      | `promptBlockKindUser`            | new `promptBlockKindUser`      |
| `RoleAssistant` | `promptBlockKindAssistant`       | new `promptBlockKindAssistant` |

Note that `RoleTool` does *not* open a new block — it only joins an existing user block (or opens one). This is what merges `RoleUser` + `RoleTool` and `RoleTool` + `RoleUser` adjacencies into a single Anthropic user message.

`buildParams` then iterates `[]promptBlock` instead of `opts.Prompt`:

```go
blocks := groupIntoBlocks(opts.Prompt)
for _, block := range blocks {
    switch block.kind {
    case promptBlockKindSystem:    // emit BetaTextBlockParams into p.System
    case promptBlockKindUser:      // emit ONE BetaMessageParam{Role: "user", ...}
    case promptBlockKindAssistant: // emit ONE BetaMessageParam{Role: "assistant", ...}
    }
}
```

**Why a flat slice rather than a typed sealed interface?** The Go port has consistently preferred flat structs with discriminator fields (per `AGENTS.md` "Typed string enums for discriminator fields", and per the recent `2026-04-30-lossless-provider-wire` change that collapsed sealed interfaces into discriminated structs). `promptBlock` is internal, package-private, and short-lived; an iota discriminator with a switch is the lightest idiomatic form.

**Alternative considered: thread `groupIntoBlocks` directly into `buildParams` as a nested loop without an intermediate slice.** Rejected because it makes the per-block emission harder to test and harder to read. A small slice up front is cheap and isolates the grouping logic so it can be tested independently if needed.

### D2. Extend `convertUserContent` to handle `ContentPartTypeToolResult` and skip approval responses

`convertUserContent` gains a new switch arm that dispatches `ContentPartTypeToolResult` to the same helper used by `convertToolContent` (see D3). It also gains a no-op skip for `ContentPartTypeToolApprovalResponse` so a future producer that emits approval responses on `RoleUser` does not silently drop them as warnings.

The function signature stays `func convertUserContent(v *cacheControlValidator, parts []provider.ContentPart, msgOpts provider.ProviderOptions, betas *[]anthropic.AnthropicBeta) []anthropic.BetaContentBlockParamUnion` — *plus* the extra parameters that `convertToolContent` needs (`mcpToolUseIDs map[string]bool`, `warnings *[]provider.Warning`). All four parameters are already in scope at the call site in `buildParams`.

**Why route through the same helper rather than duplicate?** Tool-result blocks have non-trivial branching: standard tool-result vs MCP-tool-result, output-type-driven content serialization, error flag propagation. Duplicating that logic across two functions is the bug we're paying down here — the original `convertUserContent` was a stripped-down tool-result-free copy of the user-handling branch and drifted from upstream. A single helper prevents future re-divergence.

### D3. Extract a shared `appendToolResultBlock` helper from `convertToolContent`

The current `convertToolContent` has the inner block-building logic inlined. We extract it into:

```go
func appendToolResultBlock(
    blocks []anthropic.BetaContentBlockParamUnion,
    p provider.ContentPart,
    cc anthropic.BetaCacheControlEphemeralParam,
    mcpToolUseIDs map[string]bool,
    warnings *[]provider.Warning,
) []anthropic.BetaContentBlockParamUnion
```

The helper takes the resolved cache-control (cache-control resolution stays in the caller, because the cascade rule depends on whether the part is the last part of its *source provider message*, not the last part of the merged Anthropic block — see D4). It chooses between `OfMCPToolResult` (when `mcpToolUseIDs[p.ToolCallID]` is true) and `OfToolResult` exactly as today.

`convertToolContent` becomes a thin loop that resolves cache-control per part and calls `appendToolResultBlock`. `convertUserContent` calls the same helper from its `ContentPartTypeToolResult` branch.

**Why pass `blocks` as a value parameter and return the new slice (vs `*[]...`)?** Idiomatic Go for builder helpers; matches `appendBetaUnique` already in this file. The append-and-return pattern composes cleanly with the existing `blocks = append(blocks, ...)` style elsewhere in the converter.

### D4. Cache-control cascade is per-source-message, not per-merged-block

Upstream treats the merged user-block's cache-control cascade carefully (`packages/anthropic/src/convert-to-anthropic-prompt.ts:130-145, 326-338`):

- For a part inside a `user` source message, "is last part" means *last part of that source message's `content` slice* — not last part of the entire merged Anthropic block.
- The fallback that picks up `message.providerOptions` only applies to the last part of the *source message*.

The Go port already has the right primitive for this (`v.resolveCacheControl(partOpts, msgOpts, isLast, canCache)` in `cache_control.go`), and the existing `convertUserContent` and `convertToolContent` already drive `isLast` off the per-source-message slice index. We preserve that behavior: the per-block emission loop iterates each source `provider.Message` in the block, and inside that inner loop `isLast := j == len(msg.Content)-1` is keyed off `msg.Content`, not the merged-block content.

This means a multi-message user block can produce *multiple* "last parts" of separate source messages, each with its own message-level cache-control cascade. That matches upstream semantics. It also means the cache-control budget (4 breakpoints, enforced by `cacheControlValidator.breakpoints`) keeps incrementing across the merge — same as upstream, because upstream reuses one validator per request.

**Alternative considered: collapse cache-control resolution to "last part of merged block".** Rejected because it would silently drop legitimate cache-control hints set on intermediate-source-message parts. Producers that set cache-control on a tool-result message and then send a separate text follow-up message would lose the tool-result cache breakpoint after the merge.

### D5. Multi-message assistant block handling

`groupIntoBlocks` may return an `AssistantBlock` carrying several consecutive `RoleAssistant` messages. The current per-message loop in `buildParams` already produces a single Anthropic message per source message; under the new grouping it must produce a single merged Anthropic message per *block*. We follow upstream behavior (`convert-to-anthropic-prompt.ts:493-500, 539-548`):

- Concatenate every source message's content into one `[]anthropic.BetaContentBlockParamUnion`.
- Trailing-whitespace trim is applied only when the part is *the last text part of the last message of the last block* (Anthropic disallows trailing whitespace on a prefilled assistant turn).

The Go port currently trims trailing whitespace at... actually, let me re-read. Looking at `convertAssistantContent` in `anthropic/convert_request.go`, the Go port does *not* trim trailing whitespace at all today. Upstream does. That is a separate (pre-existing) parity gap not covered by #173 — calling it out as a non-goal here so the implementer doesn't bundle it. The merged-block emission preserves the *current* Go behavior on text trimming. If a follow-up issue ports the trim, it can layer on top.

**Why preserve current behavior for assistant trim rather than fix it now?** Scope discipline. #173 names two gaps (`convertUserContent` drops tool-result, no `groupIntoBlocks`); the assistant-trim gap is a separate parity issue that warrants its own change with its own tests.

### D6. Multi-message system block handling

`groupIntoBlocks` may return a `SystemBlock` carrying several `RoleSystem` messages. Upstream raises `UnsupportedFunctionalityError` if multiple system messages are *interleaved* with user/assistant messages; with consecutive system messages it concatenates their content into a single `system` array. The Go port currently appends each system message's text as a separate `BetaTextBlockParam` to `p.System` regardless of position — which is more permissive than upstream.

We preserve current Go behavior for this change (append to `p.System` per source message), because:

- `groupIntoBlocks` already prevents a *resumed* system block (i.e. system messages separated by other roles) from concatenating cleanly — they would land in two separate `SystemBlock` entries in the slice. The looser Go behavior only differs in whether we raise an error for the interleaved case; it does not affect #173's wire correctness.
- Raising an unsupported-functionality error is a behavior change orthogonal to the user/tool grouping fix. It can be a follow-up.

### D7. ProviderOptions on the merged user block

There is no merged-block-level `ProviderOptions` in either upstream or the Go port — neither emits provider-options at the `BetaMessageParam` level (Anthropic's API doesn't have a slot for it on a user message). The cascade rule in D4 is the only place message-level `ProviderOptions` matter, and it's keyed off the *source* message. So the merge is invisible from the cache-control's perspective; nothing additional is required.

## Risks / Trade-offs

- **Risk: existing tests assume one Anthropic message per `provider.Message` and break under the new grouping.** Mitigation: audit `convert_request_test.go`. Specifically, any test that constructs adjacent `RoleUser` + `RoleTool` (or `RoleTool` + `RoleUser`) expecting two `p.Messages` entries will need to be updated to expect one merged entry. None of the production tests in the current file do this (consecutive same-role merges aren't exercised today), so the audit should be cheap; if anything is found it's strong evidence the test was masking the bug we're fixing.
- **Risk: cache-control breakpoint count interacts subtly with the merge.** Mitigation: explicit unit test that constructs a `RoleTool(tool_result, msgOpts=ephemeral)` followed by a `RoleUser(text, partOpts=ephemeral)` and asserts both breakpoints land — i.e. the validator counts each correctly across the merge boundary.
- **Risk: a producer relies on `RoleTool` in the middle of a sequence opening a fresh user message (e.g. to inject a divider).** Mitigation: this is an upstream-divergent behavior; aligning with upstream is the explicit goal of #173. Document the behavior change in the PR description.
- **Risk: ordering bug (text appears before tool-result instead of after, or vice versa).** Mitigation: the implementation iterates source messages in order and parts within each message in order, with `append` to the merged-content slice — preserves order by construction. Add an explicit ordering test: `RoleUser([tool_result, text])` produces `[tool_result, text]` on the wire; `RoleTool([tool_result]) + RoleUser([text])` produces `[tool_result, text]` on the wire.
- **Trade-off: introduces an extra slice allocation per request (the `[]promptBlock`).** Mitigation: prompts have small message counts (typically <50); the allocation is negligible relative to the per-message JSON marshaling cost.

## Migration Plan

This is a pure bug fix with no API surface change and no wire-format change. It lands as a single PR per the `git status` task. No migration steps are required for downstream consumers; the workaround in `grafana/grafana-assistant-app` Lodestone (`MessagesFromAnthropic` splitting mixed user messages) becomes a no-op once the fix is in and can be cleaned up in a follow-up there.

Rollback: revert the PR. Behavior reverts to "one Anthropic message per provider message; tool_result parts in user messages dropped silently". Downstream consumers must re-apply the workaround.

## Open Questions

- **None.** The upstream behavior is well-defined and the issue description spells out the desired Go behavior. The four scope-bounded parity gaps not covered by this change (assistant trailing-whitespace trim; multi-system-message interleave error; the broader #171/#172 work) are all explicitly enumerated in the proposal as out of scope.
