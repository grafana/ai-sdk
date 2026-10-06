## Context

The `provider/` package was originally based on an earlier version of the upstream Vercel AI SDK spec. The current upstream V3 and V4 specs are identical for the types changed here (Usage, FinishReason, ResponseMetadata), meaning our Go port has been behind upstream for some time. This change (PR 1 of 3 from #32, issue #66) reshapes the core type foundations to match current upstream semantics. It must land first because PR 2 (content model expansion) and PR 3 (tool types) depend on these shapes.

Current state:
- `Usage` is flat with optional detail sub-structs (`InputTokenDetails`, `OutputTokenDetails`). `TotalTokens` exists but is never populated. Upstream V3/V4 uses nested `inputTokens`/`outputTokens` structs.
- `FinishReason` is a `string` type alias. `RawFinishReason` is a separate field on `StreamPart`. Upstream V3/V4 uses a struct with `unified` + `raw` fields at the provider level.
- The type is called `Metadata` but every field using it is named `ProviderMetadata` (110+ sites reference the field name).
- `ResponseMetadata` includes `Headers` and `Body`, but `Body` is never set by any provider and `Headers` is only set in the streaming path. Upstream V3/V4 slims this to `{id, timestamp, modelId}` with headers/body at result level.
- Two separate `ResponseMetadata` types exist: provider-level (with Headers/Body) and root-level (with Headers/Messages).

## Goals / Non-Goals

**Goals:**
- Align provider-level types with upstream V4 semantics
- Maintain SSE wire compatibility with `@ai-sdk/react` (UIMessageChunk format unchanged)
- Keep the codebase compilable within a single PR (all consumers updated atomically)

**Non-Goals:**
- New content types (CustomContent, ReasoningFile, ToolApprovalResponse) -- that's PR 2 (#67)
- Tool type split (FunctionTool/ProviderTool) -- that's PR 3 (#68)
- Warning type changes -- PR 2
- Go API backward compatibility (no deprecation aliases)
- Changing the root-level `ResponseMetadata` in `text.go` -- that type serves the orchestration layer and has different semantics (includes `Messages`)

## Decisions

### D1: Usage restructure -- nested sub-structs replace flat fields + detail types

Collapse `InputTokens *int` + `InputTokenDetails` into a single `InputTokenUsage` struct. Same for output. Delete `TotalTokens` (never populated, consumers can derive it).

```
Usage {
    InputTokens  InputTokenUsage
    OutputTokens OutputTokenUsage
    Raw          json.RawMessage
}

InputTokenUsage  { Total, NoCache, CacheRead, CacheWrite *int }
OutputTokenUsage { Total, Text, Reasoning *int }
```

**Why this over keeping flat fields**: Matches upstream V4 exactly. The flat+details pattern forced nil-checking two levels (`Usage.InputTokenDetails != nil && Usage.InputTokenDetails.CacheReadTokens != nil`). The nested approach is one level of nil-checking on the inner `*int` fields.

**Migration of `aggregateUsage`**: Currently sums `InputTokens` and `OutputTokens` top-level `*int` fields. Must change to sum `InputTokens.Total` and `OutputTokens.Total`. Detail fields (cache, reasoning) are not aggregated across steps (same as current behavior where details are ignored in aggregation).

### D2: FinishReason -- from string alias to struct with Unified + Raw

Replace `type FinishReason string` with a struct. Introduce `UnifiedFinishReason` typed string for the constants.

```
type UnifiedFinishReason string
const FinishReasonStop UnifiedFinishReason = "stop"  // etc.

type FinishReason struct {
    Unified UnifiedFinishReason
    Raw     string
}
```

Remove `StreamPart.RawFinishReason` -- folded into `FinishReason.Raw`. Also remove `RawFinishReason` from orchestration types (`StepResult`, `StreamFinishStep`, `StreamFinish`) -- the struct carries both values.

**Why `UnifiedFinishReason` as a separate type**: Keeps the existing constant names (`FinishReasonStop`, etc.) usable and type-safe. The struct `FinishReason` is what flows through the system; `UnifiedFinishReason` is the enum for the `Unified` field.

**Divergence from upstream's two-type pattern**: The upstream has two distinct FinishReason representations -- a struct `{ unified, raw }` at the provider level, and a simple string union at the orchestration level (`StepResult`, `TextStreamPart`, etc.). At the boundary (`stream-model-call.ts`), the upstream destructures the struct into `finishReason` (string) + `rawFinishReason` (string) as separate fields. We choose to keep a single `FinishReason` struct flowing through all layers instead. This is simpler in Go (one type, no field duplication) and avoids the need to destructure/reassemble at boundaries. The trade-off is divergence from upstream's internal layering, but the wire format and external behavior are identical.

**SSE wire format**: `UIMessageChunk.FinishReason` stays as `string`. We serialize `FinishReason.Unified` into it. No wire-format change.

**Anthropic mapping**: `mapFinishReason` returns `FinishReason` (struct) instead of `FinishReason` (string). Takes the raw provider string as a second input to populate `Raw`.

### D3: Metadata type rename to ProviderMetadata

Rename `type Metadata map[string]json.RawMessage` to `type ProviderMetadata map[string]json.RawMessage`.

**Why a simple rename**: Every field in the codebase already uses the name `ProviderMetadata`. Only the type definition says `Metadata`. This is a mechanical find-and-replace of the type name. Also rename `GenerateResult.Metadata` field to `GenerateResult.ProviderMetadata` to match the naming convention used everywhere else.

### D4: ResponseMetadata slimmed + GenerateResponse for result-level

Slim `provider.ResponseMetadata` to `{ID, Timestamp, ModelID}` only. Create a new `GenerateResponse` struct that embeds `ResponseMetadata` and adds `Headers` and `Body` for `GenerateResult.Response`.

```
type ResponseMetadata struct { ID, ModelID string; Timestamp time.Time }

type GenerateResponse struct {
    ResponseMetadata
    Headers map[string]string
    Body    json.RawMessage
}

type GenerateResult struct {
    ...
    Response *GenerateResponse  // was *ResponseMetadata
}
```

`StreamResult.Response` stays as `*ResponseHeaders` (just headers) -- unchanged, matching V4.

**Why `GenerateResponse` as a named type**: Upstream V4 defines the result-level response as `ResponseMetadata & { headers, body }`. Go embedding gives us the same composition. A named type is cleaner than inlining fields on `GenerateResult` and allows for future extension.

**Root-level `ResponseMetadata` (text.go) untouched**: The orchestration-level type has different semantics (includes `Messages` for conversation history). It's not part of the provider interface contract.

### D5: specificationVersion bump

Change the `specVersion` constant in `anthropic/model.go` from `"v3"` to `"v4"`. Update all test mocks that return `"v3"`.

## Risks / Trade-offs

**[Large blast radius on ProviderMetadata type rename]** 110+ sites reference the `Metadata` type. Since every field is already named `ProviderMetadata`, the rename is mechanical but touches many files. -> Mitigation: Use IDE/sed rename. All breakages are compile errors (type not found), so the compiler catches everything.

**[FinishReason struct makes construction more verbose]** Code like `part.FinishReason = provider.FinishReasonStop` becomes `part.FinishReason = provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "..."}`. -> Mitigation: This is the correct V4 behavior. Could add a helper `provider.NewFinishReason(unified, raw)` if verbosity is excessive, but probably unnecessary for the few construction sites.

**[Two ResponseMetadata types still exist after this change]** The provider-level one gets slimmed, but the root-level one (text.go) keeps its own shape. -> Mitigation: These serve different purposes. The root-level type is the orchestration output, not the provider contract. Aligning it is separate work if needed.

**[aggregateUsage must handle nil nested structs correctly]** With the nested structure, `InputTokens` is no longer a `*int` but a struct. Zero-value struct with nil `*int` fields must be handled. -> Mitigation: Check `Total != nil` before dereferencing, same pattern as current nil-check on `*int`.
