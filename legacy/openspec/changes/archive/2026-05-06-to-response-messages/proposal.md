## Why

Issue #171: `appendToolResults` (`streamtext.go:1392`) builds the next-call assistant
message from `step.Text` + `step.ToolCalls` only, dropping every reasoning block and its
`ProviderMetadata`. On Anthropic with extended thinking, this strips the `signature` field
that the API expects when the conversation continues, breaking chain-of-thought continuity
and silently degrading multi-step tool-use quality.

> "After the first tool round, the next-call assistant message has no reasoning parts and
> no signatures." — #171

Issue #172: Upstream exposes `toResponseMessages`
(`packages/ai/src/generate-text/to-response-messages.ts`) as the canonical
"collected stream content -> next-call messages" helper, also surfaced as
`result.response.messages`. The Go port has no equivalent. Consumers driving their own
multi-step loops (Lodestone in `grafana-assistant-app` ships ~450 LOC of stream
accumulator + content-to-message converter purely to work around this) cannot build the
next-call message list correctly and must reimplement the conversion.

> "Internally, `ToResponseMessages` and `appendToolResults` should share the same
> conversion logic. The unexported `appendToolResults` becomes a thin adapter over the
> public helper." — #172

This change resolves both issues with one fix because #172 introduces the public helper
that subsumes #171's bug fix: routing the conversion through a single upstream-aligned
function naturally preserves reasoning across rounds.

## What Changes

- **NEW PUBLIC API**: `ToResponseMessages(parts []provider.ContentPart) []provider.Message`. (The original proposal included a `tools ToolSet` parameter for upstream-API parity; final review dropped it because the Go port runs `Tool.ToModelOutput` eagerly during execution and stores the result on `ToolResult.ModelOutput`, so the parameter would have been unused. Public callers needing late-binding output conversion can call `Tool.ToModelOutput` directly before constructing parts.)
  in a new file `to_response_messages.go`. Mirrors upstream `toResponseMessages` behavior:
  - `text` -> assistant text part (empty text dropped)
  - `reasoning` -> assistant reasoning part with `ProviderOptions` carried from `ProviderMetadata` (the **#171 fix**)
  - `reasoning-file` / `file` -> assistant message
  - `tool-call` -> assistant tool-call part (invalid+non-object input collapsed to `{}`)
  - provider-executed `tool-result` -> inline in assistant message
  - non-provider-executed `tool-result` -> separate tool message
  - `tool-approval-response` -> tool message; if `Approved == false`, also adds an
    `execution-denied` synthetic tool result for the matching call
  - `custom` -> assistant message
  - sources / empty text -> skipped
- **REFACTOR**: `appendToolResults` in `streamtext.go` becomes a thin adapter that builds
  a `[]provider.ContentPart` from the step (in stream order: reasoning, text, tool-calls
  with inline provider-executed results, then non-provider-executed tool-results) and
  delegates to `ToResponseMessages`. The reasoning-drop bug is fixed by construction.
- **NEW FIELD**: re-introduce `Messages []provider.Message` on `aisdk.ResponseMetadata`
  with a `json:"-"` tag (matching the previous field shape removed in
  `2026-04-08-cleanup-metadata-and-convert`). After every step, the result's
  `step.Response.Messages` SHALL be populated by calling `ToResponseMessages` over that
  step's content. `result.Response().Messages` SHALL hold the messages corresponding to
  the last step. This mirrors upstream `result.response.messages` and lets consumers
  retrieve the next-call message list without re-invoking the helper.
- **NEW UNIT TESTS** in `to_response_messages_test.go` covering the upstream test cases
  ported to Go: text only, text + tool-call, tool-result -> tool message, tool-error
  output, reasoning + signature preserved, redacted reasoning, multipart tool result
  via `ModelOutput`, file/reasoning-file ordering, empty text dropped, empty content
  produces empty result, provider-executed inline, tool-approval-response paths,
  invalid tool-call sanitization.
- **NEW REGRESSION TESTS** in `streamtext_test.go` asserting reasoning + signatures
  survive multi-step calls (the **#171 regression guard**) and that
  `appendToolResults` still routes provider-executed vs non-provider-executed correctly
  (existing tests preserved).

## Capabilities

### New Capabilities

- `to-response-messages`: a public helper that converts collected step content parts
  into the assistant + tool messages to feed into the next call, and surfaces the
  result on the per-step `Response.Messages` field so consumers can retrieve it
  without re-invoking the helper.

### Modified Capabilities

- `provider-executed-tool-roundtrip`: the existing requirement on `appendToolResults`
  is updated to delegate to `ToResponseMessages`, and to preserve reasoning content
  parts across tool-result rounds. The provider-executed routing behavior is
  unchanged in observable shape but is now expressed in terms of the public helper.

## Impact

- **Files touched (root `aisdk/` Go module only)**:
  - `to_response_messages.go` *(new)* — public helper.
  - `to_response_messages_test.go` *(new)* — unit tests ported from upstream.
  - `streamtext.go` — refactor `appendToolResults` to delegate; populate
    `step.Response.Messages` after each step; thread the conversion through
    the existing `providerMetadataToOptions` and `toolResultOutput` helpers.
  - `text.go` — re-add `Messages []provider.Message` field on `aisdk.ResponseMetadata`
    with `json:"-"` (in-process only, not on the wire).
  - `streamtext_test.go` — add reasoning-preservation regression tests; existing
    `TestAppendToolResults_ProviderExecutedRouting` cases keep passing through the
    refactor.
- **Public API surface**: adds one new symbol (`ToResponseMessages`) and re-adds
  one struct field (`ResponseMetadata.Messages`). No existing symbol changes shape.
- **Wire format**: unchanged. `Messages` is `json:"-"` (in-process only), matching
  upstream's `result.response.messages` semantics. SSE chunk types and field names
  are untouched, so the integration-test harness in `test/integration/` is not
  affected.
- **Downstream impact**: `grafana-assistant-app` Lodestone's local stream
  accumulator + content-to-message converter
  (`internal/agentic/lodestone/aisdk/{stream_accumulator,messages_reverse}.go`)
  becomes redundant once consumers can call `ToResponseMessages` directly or read
  `result.Response().Messages`. Removal of that workaround is downstream work
  outside this PR.
- **Anthropic submodule (`anthropic/`)**: untouched. A sibling change covers
  unrelated user/tool-result grouping work for #173.
- **Closes #171**, **closes #172**.
- **Suggested PR title**: `feat(ai): public ToResponseMessages helper preserves reasoning across tool-result rounds`.
