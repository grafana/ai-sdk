## Context

Upstream commit `ad0b376` (PR vercel/ai#14542) replaced the `fine-grained-tool-streaming-2025-05-14` beta header with a per-tool `eager_input_streaming` parameter, defaulted to `true` for function tools when the request is streaming. The per-tool default is gated by a model-level `toolStreaming` option (default `true`), so callers can opt out globally for a model, and individual tools can still override via `providerOptions.anthropic.eagerInputStreaming`.

Current state of the Go port:
- `AnthropicOptions` in `providers/anthropic/options.go` does NOT have a `ToolStreaming` field.
- `AnthropicToolOptions.EagerInputStreaming *bool` already exists and is honored by `convertTools` in `providers/anthropic/convert_request.go:1436-1438`.
- The Anthropic SDK type `BetaToolParam.EagerInputStreaming` is a `param.Opt[bool]` that we set via `anthropic.Bool(...)`.
- `buildParams` is called from both `DoStream` and `DoGenerate` in `providers/anthropic/model.go` and has no awareness of streaming vs. non-streaming.
- The `fine-grained-tool-streaming-2025-05-14` beta has never been emitted (verified by grep).

## Goals / Non-Goals

**Goals:**
- Match upstream user-observable behavior: on `DoStream`, function tools default to `eager_input_streaming: true` unless either the tool or the model opts out.
- Preserve all existing behavior on `DoGenerate` and for explicit per-tool overrides.
- Keep the change additive — no breaking API change, no wire change for `@ai-sdk/react`.

**Non-Goals:**
- Removing or adding any beta header (Go port never had `fine-grained-tool-streaming-2025-05-14`).
- Changing behavior for provider-defined / server / MCP tools — `eager_input_streaming` only applies to custom function tools, as in upstream.
- Adding telemetry, retry hooks, or other unrelated streaming features.

## Decisions

### 1. Threading the `stream` signal through `buildParams`

The defaulting decision lives in `convertTools`, but `convertTools` currently has no way to know whether the request is streaming. Three options were considered:

- **A. Add a `stream bool` parameter to `buildParams` and forward to `convertTools`.** Direct, mirrors upstream's `stream` plumbing, no global state. Chosen.
- **B. Add a method-level flag on the model.** Would couple the model struct to a transient request property. Rejected.
- **C. Default eager streaming at the model layer (in `DoStream` after `buildParams`).** Would require post-processing `p.Tools`, walking the typed union to find function tools and mutating `EagerInputStreaming`. More code, easier to drift from upstream. Rejected.

**Decision: A.** `buildParams` gains a `stream bool` parameter; `convertTools` gains a `defaultEagerInputStreaming bool` parameter (the resolved boolean, computed once in `buildParams` from `stream && resolveToolStreaming(anthropicOpts)`).

### 2. `ToolStreaming` field shape

Upstream uses `toolStreaming?: boolean` with `?? true` semantics. In Go we need to distinguish "unset" from "false":

- **A. `ToolStreaming *bool` with `nil` → true.** Matches upstream exactly. Chosen.
- **B. `ToolStreaming bool` with default true.** Cannot distinguish unset from `false` because zero value of `bool` is `false`. Rejected.
- **C. A custom enum type.** Overkill for a binary flag. Rejected.

**Decision: A.** New field: `ToolStreaming *bool \`json:"toolStreaming,omitempty"\``. JSON field name matches upstream camelCase.

### 3. Where to compute `defaultEagerInputStreaming`

Two options:

- **A. Compute in `buildParams`, pass as already-resolved `bool` to `convertTools`.** Keeps `convertTools` focused on tool conversion. Chosen.
- **B. Compute inside `convertTools` from raw options.** Couples `convertTools` to model-level option parsing, which it currently does not do. Rejected.

**Decision: A.** `buildParams` resolves the anthropic-level options once and computes the effective default before calling `convertTools`.

### 4. Per-tool override semantics

Upstream:
```ts
const eagerInputStreaming =
  anthropicOptions?.eagerInputStreaming ?? defaultEagerInputStreaming;
// ...
...(eagerInputStreaming ? { eager_input_streaming: true } : {}),
```

So an explicit `false` on a tool wins over the model-level `true` default, and an explicit `true` wins over a model-level `false`. Critically, the field is only emitted when the resolved value is truthy: `false` (whether from the model-level default or an explicit per-tool override) suppresses the field entirely rather than sending `eager_input_streaming: false`.

Go mirrors this by resolving the effective value first (`toolOpts.EagerInputStreaming != nil ? *toolOpts.EagerInputStreaming : defaultEagerInputStreaming`) and only assigning `tp.EagerInputStreaming = anthropic.Bool(true)` when the resolved value is truthy. The Anthropic SDK's `param.Opt[bool]` semantics are such that unset values are omitted from the JSON body, achieving exact upstream wire parity.

### 5. JSON response-format fallback tool

Upstream's `getArgs` calls `prepareTools` exactly once, appending the synthetic `json` fallback tool to the user-supplied tools list when `responseFormat.type === 'json'` and the model lacks native structured-output support (anthropic-language-model.ts:519-545). The same `defaultEagerInputStreaming` is passed, so the JSON fallback tool participates in the per-tool default just like any user-provided function tool — upstream test snapshots in PR vercel/ai#14542 confirm `"eager_input_streaming": true` on the `"Respond with a JSON object."` tool.

The Go port has historically split this responsibility: `convertTools` produces `p.Tools` from user tools, and `applyResponseFormat` separately appends the `json` fallback tool to `p.Tools`. To preserve that structure while matching upstream behavior, we thread `defaultEagerInputStreaming` into `applyResponseFormat` and set `EagerInputStreaming` on the synthetic tool directly when the flag is truthy. The fallback tool has no `ProviderOptions["anthropic"]`, so no per-tool override resolution is needed.

### 6. Behavior when there are no function tools

When all tools are provider-defined / server tools, no `eager_input_streaming` field is emitted. This matches upstream because `prepareTools` only writes the field on the custom function-tool branch.

## Risks / Trade-offs

- **Risk:** Existing callers who rely on the implicit "no eager streaming unless explicitly opted-in" behavior on `DoStream` will see eager streaming kick in.
  - **Mitigation:** This is the desired alignment with upstream. Callers who want to disable can set `AnthropicOptions.ToolStreaming = anthropic.BoolPtr(false)` (or per-tool `false`). Mention in the change description; no API break.
- **Risk:** A tool that previously set `EagerInputStreaming` to `false` explicitly must continue to win over the default.
  - **Mitigation:** Per-tool override is checked first (`if toolOpts.EagerInputStreaming != nil`), as in upstream. Covered by a unit test.
- **Risk:** Drift between `DoStream` and `DoGenerate` if a future code path forgets to forward the `stream` flag.
  - **Mitigation:** Single call site for each in `model.go`; covered by tests asserting that `DoGenerate` paths (and `buildParams(..., stream=false)`) never set `eager_input_streaming` from the default.

## Migration Plan

No migration is required. Behavior on `DoGenerate` is unchanged. Behavior on `DoStream` becomes more aggressive by default — callers wanting the old behavior set `ToolStreaming: anthropic.BoolPtr(false)` in `AnthropicOptions`, or set `EagerInputStreaming: anthropic.BoolPtr(false)` on each tool.

## Open Questions

None.
