## Context

The Go port of Vercel's AI SDK currently has no public equivalent of upstream
`toResponseMessages` — the helper that converts collected response content parts
into the `[]Message` to feed into the next call. The closest internal function
is `appendToolResults` (`streamtext.go:1392`):

- It is unexported and tightly coupled to `StreamText`'s tool-execution loop.
- It walks `step.Text` + `step.ToolCalls` only, so reasoning blocks (with their
  Anthropic `signature` ProviderMetadata) drop on every tool-result round (#171).
- It does its own routing for provider-executed tool results (kept inline) vs
  non-provider-executed (separate tool message) — logic that any external
  consumer that wants to drive multi-step calls themselves must rewrite.

Issue #172 asks for the public helper. Issue #171 is the bug fix it implies.
This change combines both: the refactor naturally fixes the reasoning drop.

The upstream reference is
`packages/ai/src/generate-text/to-response-messages.ts`. Its tests in
`to-response-messages.test.ts` enumerate the behavior we must mirror. Because
this repo is a Go port of `vercel/ai`, "upstream parity" means matching
*behavior*, not *code shape* — see `.cursor/rules/upstream-parity.mdc`.

The previous change `2026-04-08-cleanup-metadata-and-convert` removed an unused
`Messages` field from `aisdk.ResponseMetadata`, with this explicit note:

> "If upstream alignment requires `toResponseMessages()` later, the field must
> be re-added. -> Acceptable; dead code shouldn't stay to anticipate potential
> future use (YAGNI). Re-adding is trivial."

That moment has arrived.

## Goals / Non-Goals

**Goals:**

- Provide a public `ToResponseMessages` helper that mirrors upstream's behavior
  for every `ContentPart` variant, including reasoning + signature preservation.
- Make `appendToolResults` a thin internal adapter over `ToResponseMessages` so
  there is exactly one source of truth for "next-call message construction"
  inside the SDK.
- Fix #171 by construction: routing the conversion through the public helper
  guarantees reasoning + `ProviderMetadata` are carried through.
- Surface the helper output on `result.Response().Messages` so consumers don't
  have to re-invoke the helper after every step.
- Keep public API churn minimal: one new symbol (`ToResponseMessages`), one
  re-introduced struct field (`ResponseMetadata.Messages`).
- Match upstream parity tests one-for-one where the Go port's type model
  supports them.

**Non-Goals:**

- A separate `aisdk.ContentPart` (sealed-interface) input shape. The function
  takes `[]provider.ContentPart` (the flat discriminated struct) directly, so
  callers don't need a parallel type tree.
- Changing the wire format. `Messages` is `json:"-"`. SSE/UI chunks are
  untouched.
- Touching the Anthropic submodule. A sibling change covers #173.
- Implementing `tool-error` and `tool-approval-request` as separate Go
  `ContentPartType` variants. The Go port already encodes errors via
  `ToolResultOutput.Type = ToolOutputErrorText/ToolOutputErrorJSON`, and the
  unimplemented `tool-approval-request` is out of scope here (no current
  provider emits it). The helper still handles `tool-approval-response` per
  upstream when that part type appears.
- Changing how `step.Content` (the public `[]aisdk.ContentPart`
  sealed-interface slice) is built. We extend orchestration with a separate
  provider-shaped content list used as input to `ToResponseMessages`.

## Decisions

### D1. Public signature: `func ToResponseMessages(parts []provider.ContentPart) []provider.Message`

The upstream signature is
`toResponseMessages({ content: ContentPart<TOOLS>[], tools: TOOLS | undefined }):
Promise<Array<AssistantModelMessage | ToolModelMessage>>`. The Go signature drops
the async wrapper (no I/O happens), drops the `tools` map (see "Why no
ToolSet parameter" below), and uses the flat discriminated
`provider.ContentPart` for both input and output:

```go
func ToResponseMessages(parts []provider.ContentPart) []provider.Message
```

**Why `provider.ContentPart` for the input?**

- The flat struct already covers every variant the helper needs: `text`,
  `reasoning`, `reasoning-file`, `file`, `tool-call`, `tool-result`, `custom`,
  `tool-approval-response`. Sources don't have a `provider.ContentPart` variant,
  which matches upstream's "skip sources" behavior exactly.
- The output type is `[]provider.Message`, which is `[]Message{Role, Content
  []ContentPart}`. Using the same struct on both sides means the helper is
  almost a structural copy — no parallel type tree, no impedance mismatch.
- Consumers driving multi-step loops already construct `provider.Message` for
  their next call; using the same shape on both sides keeps the SDK's
  conceptual surface narrow.

**Why no error return?**

Upstream's `toResponseMessages` is async only because `createToolModelOutput`
is. In the Go port the equivalent path is `toolResultOutput(tr ToolResult)`,
a pure function that already returns `*provider.ToolResultOutput`. There is no
async work, no I/O, no error path. A `(messages, error)` signature would force
every caller to handle a `nil`-error case, which is misleading. We follow the
"return what you have" Go idiom.

**Why no `ToolSet` parameter?**

Upstream takes the `tools` map so it can dispatch through `tool.toModelOutput`
lazily during message conversion. In our Go port that conversion happens
**eagerly** during tool execution: `streamtext.go` invokes the per-tool
`Tool.ToModelOutput` hook and stores the resulting `*provider.ToolResultOutput`
on `ToolResult.ModelOutput`. By the time content reaches `ToResponseMessages`,
the per-tool conversion is already done — the helper would never use the
`tools` map.

The original proposal kept the parameter for upstream API parity and as a
forward-compat seam. PR review surfaced that the seam was speculative:
`Tool.ToModelOutput` already exists on the `Tool` struct, and the eager
design has been baked in since the orchestration layer was first written.
Keeping an unused parameter would force every caller to thread `cfg.tools`
through code paths that ignore it, and the doc comment had to apologize for
the discrepancy.

The shipped signature drops the parameter. This is an intentional shape
divergence from upstream (lazy → eager) but a behavior match: every input
that produces a given output upstream produces the same output here. Public
callers who construct parts from raw tool output and need the late-binding
behavior can call `Tool.ToModelOutput` directly before constructing the
`provider.ContentPart` — the hook is already public.

### D2. Internal architecture: one builder, two callers

```
                                           ┌─────────────────────────────┐
                                           │ Public consumers driving    │
                                           │ multi-step loops themselves │
                                           └──────────────┬──────────────┘
                                                          │
                                                          ▼
[]provider.ContentPart  ──►  ToResponseMessages  ──►  []provider.Message
                                  ▲
                                  │ (delegate)
                                  │
                          appendToolResults(msgs, step)
                                  ▲
                                  │ (called per step)
                                  │
                            StreamText.run loop
```

`appendToolResults` retains its current signature (`(msgs []provider.Message,
step StepResult) []provider.Message`). Internally it builds a
`[]provider.ContentPart` from the step (in stream order: reasoning → text →
tool-call (with inline provider-executed tool-result) → other tool-results),
then calls `ToResponseMessages` and appends the returned messages onto `msgs`.

Building the parts list is the only step-shape-aware code; the conversion
itself is a single switch-on-`Type` in `ToResponseMessages`. This is the
"share inner builder via a small helper" approach the issue suggests. We do
not split `ToResponseMessages` into two functions: the upstream contract is one
function, and Go callers benefit from getting both messages back as a single
slice.

### D3. Per-variant behavior maps 1:1 to upstream

Switch arms in `ToResponseMessages` (using `ContentPartType` typed string
constants per `AGENTS.md`):

| Variant                          | Behavior                                                                                                     |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `text`, empty `Text`             | skipped                                                                                                      |
| `text`                           | assistant `text` part with `ProviderOptions` from input                                                      |
| `reasoning`                      | assistant `reasoning` part with `ProviderOptions` from input (the **#171 fix**)                              |
| `reasoning-file`                 | assistant `reasoning-file` part (passes through)                                                             |
| `file`                           | assistant `file` part                                                                                        |
| `custom`                         | assistant `custom` part with `Kind` and `ProviderOptions`                                                    |
| `tool-call` (provider-executed)  | assistant `tool-call` part; if a matching `tool-result` (same `ToolCallID`) is also provider-executed, that result is appended inline immediately after the call |
| `tool-call` (client-executed)    | assistant `tool-call` part; corresponding tool-result lives in the tool message (next pass)                  |
| `tool-result` (provider-executed) | already inlined by the `tool-call` arm; skipped in the second pass                                          |
| `tool-result` (client-executed)  | tool message `tool-result` part                                                                              |
| `tool-approval-response`          | tool message; if `Approved == false`, also adds an `execution-denied` synthetic tool-result for the call    |
| sources / unknown types          | skipped                                                                                                      |

We do not implement upstream's `tool-error` or `tool-approval-request` arms —
the Go `provider.ContentPart` doesn't have those variants. Errors are encoded
on `ToolResultOutput.Type` (`error-text`, `error-json`, `execution-denied`),
which the existing `toolResultOutput(tr)` helper already produces. This is
identical to upstream wire output (the test fixtures match), just with the
sealing point one layer lower.

**Invalid tool-call sanitization.** Upstream replaces a non-object `input`
with `{}` when `invalid: true`. The Go port already does this in
`appendToolResults` via `isJSONObject(input)`. We keep the same sanitization in
`ToResponseMessages` for parity. The relevant signal in Go is the existing
`isJSONObject` helper plus the (currently dropped) `Invalid` flag — we don't
have a `provider.ContentPart`-level "invalid" bit, so the helper sanitizes
defensively whenever `Input` is non-empty and non-object.

### D4. `appendToolResults` builds parts in stream order

The order matters for Anthropic extended thinking: reasoning blocks must
precede the text/tool-use they support, otherwise the API rejects the
conversation. The Go stream emits parts in this order, but `processStep`
collects them into per-kind slices (`reasoningBlocks`, `textBuilder`,
`step.ToolCalls`, `step.ToolResults`). The current `buildContent` function
rebuilds in the order [text, tool-calls, tool-results, sources, files]. We
will not change `buildContent` (that's the public `step.Content`); instead, the
new `appendToolResults` builds a separate `[]provider.ContentPart` for the
helper input in this order:

1. `reasoning` parts from `step.Reasoning` (in collection order — already the
   stream order)
2. one `text` part from `step.Text` (only if non-empty; the helper would skip
   it anyway, but emitting cleanly avoids a confusing `[reasoning, "", call]`
   trace)
3. one `tool-call` part per `step.ToolCalls` (in collection order)
4. provider-executed `tool-result` parts paired with their matching call
   (the helper inlines these via the call-arm, so they're emitted as
   stand-alone parts here too — the call arm picks them up by `ToolCallID`)
5. non-provider-executed `tool-result` parts (the helper routes these to the
   tool message)

Files and sources are not part of the tool-result-round contract for #171/#172
— neither upstream's `appendToolResults` analog nor the current Go version
forwards them on the next call. We omit them from the helper input here. The
public `ToResponseMessages` still handles `file` parts (for callers building
their own next-call messages from a fuller content slice).

### D5. Reasoning-preservation example trace

Before the fix, an Anthropic extended-thinking + tool-use multi-step run
produces these messages on step 2's request:

```
[
  user: "What's the weather in Tokyo?",
  assistant: [tool-call(weather, {"city":"Tokyo"})],          // ← no reasoning
  tool: [tool-result(weather, "72F sunny")],
]
```

After the fix:

```
[
  user: "What's the weather in Tokyo?",
  assistant: [
    reasoning("I need to use the weather tool"   ,            // ← restored
              providerOptions={anthropic:{signature:"sig_..."}}),
    tool-call(weather, {"city":"Tokyo"}),
  ],
  tool: [tool-result(weather, "72F sunny")],
]
```

Anthropic's API accepts the second form when `thinking` is enabled; the first
form silently degrades quality (and on some account configurations, returns
400).

### D6. Surfacing on `result.Response().Messages`

Upstream exposes `result.response.messages` as the cumulative messages from the
last step (the conventional shape consumers chain through `result.toMessages()`
or feed back via `messages: [...prior, ...result.response.messages]`).

We re-add the field that the
`2026-04-08-cleanup-metadata-and-convert` change explicitly anticipated:

```go
type ResponseMetadata struct {
    provider.ResponseMetadata
    Headers  map[string]string  `json:"headers,omitempty"`
    Messages []provider.Message `json:"-"`
}
```

`json:"-"` keeps it off the wire. It exists for in-process consumers. After
`processStep` finishes a step, we populate
`step.Response.Messages = ToResponseMessages(stepContent)` where
`stepContent` is the same `[]provider.ContentPart` the orchestration layer
already built for `appendToolResults`. The two are semantically equivalent —
in fact, `appendToolResults`' output is exactly the previous-message tail
plus `step.Response.Messages`.

`result.Response()` reflects the **last** step (consistent with upstream
`result.response.messages` semantics on a single-step generate; for multi-step
streams the last step's response is what consumers care about). For
per-step access, `result.Steps()[i].Response.Messages` works.

We do not surface `Messages` on `provider.ResponseMetadata` itself — that's
upstream-pure metadata about the HTTP response. It belongs on
`aisdk.ResponseMetadata` only, where orchestration owns it.

### D7. Behavioral parity tests ported one-for-one

The upstream test file enumerates 24 cases. We port the cases the Go type model
supports directly (text, reasoning + signature, tool-call, tool-result,
multipart `ModelOutput`, file, reasoning-file, mixed ordering, empty text
dropped, empty content, provider-executed inline, tool-approval-response paths,
invalid tool-call sanitization). Cases that depend on upstream-only types
(`tool-error` as a separate variant, `tool-approval-request` as an input)
are skipped with a comment pointing back to upstream. The skipped cases
are equivalent to forms we already cover via `ToolResultOutput.Type =
error-text` and the existing approval-response routing.

## Risks / Trade-offs

- **[Risk] Adding a public symbol enlarges the SDK API surface.** → The single
  new symbol (`ToResponseMessages`) directly mirrors upstream and is
  load-bearing for any consumer driving their own multi-step loops; the
  alternative is every consumer reimplementing this themselves (already
  happening in Lodestone). The benefit clearly outweighs the surface cost.

- **[Risk] `ResponseMetadata.Messages` re-introduces a field that was
  intentionally removed.** → The previous removal explicitly anticipated this
  re-introduction. The field is `json:"-"`, costs nothing on the wire, and
  consumers can ignore it.

- **[Trade-off] `appendToolResults` now does a tiny extra allocation** (the
  `[]provider.ContentPart` it passes to the helper). → Negligible compared to
  the cost of the model call itself; not in any hot path.

- **[Trade-off] `ToResponseMessages` doesn't return an error.** → Upstream is
  async only because `createToolModelOutput` is async; in Go the equivalent
  is pure. If we ever add async work (e.g., remote tool output normalization),
  we'd need a v2 signature. Acceptable: we'd sign that change separately under
  the same upstream-parity rule.

- **[Risk] Stream order in `step.Reasoning` may not perfectly match the order
  text + tool-calls were emitted in.** → The Anthropic provider always emits
  reasoning blocks before text/tool-use blocks per the API contract. For other
  providers, the worst case is a reasoning block ending up after text in the
  next-call message; Anthropic is the only provider where this can break the
  conversation, and Anthropic's ordering is correct. We accept this and add a
  TODO if a future provider needs strict interleaving.

## Migration Plan

This is a non-breaking change:

- New public symbol `ToResponseMessages` — additive.
- New struct field `ResponseMetadata.Messages` (json-ignored) — additive.
- Internal refactor of `appendToolResults` — caller-invisible; existing
  `TestAppendToolResults_ProviderExecutedRouting` cases keep passing.

No deprecations, no codemods. The only thing downstream consumers
(`grafana-assistant-app` Lodestone) need is a follow-up PR to delete their
local stream accumulator + `messages_reverse` converter, replacing those
~450 LOC with a call to `ToResponseMessages` (or reading
`result.Response().Messages`). That removal is downstream-only and out of
scope for this change.

## Open Questions

None. The signature, behavior, and surfacing are fully specified by upstream.
