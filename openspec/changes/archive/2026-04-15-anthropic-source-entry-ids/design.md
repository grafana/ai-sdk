## Context

The Anthropic provider emits source entries in two paths:

1. **Streaming** (`DoStream` → `consumeStream` → `emitWebSearchResult`): Builds `provider.SourceInfo` and sends it on the stream channel. The `SourceInfo` struct already has an `ID` field, but `emitWebSearchResult` does not populate it. The `citations_delta` path already sets `ID` correctly via `createCitationSource`.

2. **Non-streaming** (`DoGenerate` → `convertResponse`): Builds `provider.GenerateContentPart` entries for citations and web search results. The `GenerateContentPart` struct has no `ID` field, so even though `createCitationSource` returns a `SourceInfo` with `ID` set, the ID is dropped when mapping to `GenerateContentPart`.

Upstream TypeScript always sets `id: generateId()` on every source entry in both paths.

## Goals / Non-Goals

**Goals:**

- Every source entry emitted by the Anthropic provider carries an `ID`, matching upstream behavior.
- Streaming and non-streaming paths are consistent with each other.
- Wire format for non-streaming `GenerateResult` content parts of type `source` includes `"id"` in JSON.

**Non-Goals:**

- Changing how `generateID` is configured or overridden (no new options).
- Modifying other providers -- only the Anthropic provider is in scope.
- Adding ID to non-source content part types.

## Decisions

### 1. Add `ID` field to `GenerateContentPart`

Add `ID string` with JSON tag `"id,omitempty"` to `provider.GenerateContentPart`. This is the minimal struct change that lets non-streaming source parts carry an identifier.

**Alternative considered**: Create a separate `GenerateSourcePart` type. Rejected because `GenerateContentPart` is already a flat union keyed by `Type`, and adding a field is simpler and consistent with how other variant-specific fields (e.g., `SourceType`, `URL`, `MediaType`) are handled.

### 2. Populate ID in streaming web search sources

In `emitWebSearchResult`, set `ID: a.generateID()` on each `SourceInfo`. The `generateID` function is already available on the `streamAdapter` struct.

### 3. Populate ID in non-streaming paths

In `convertResponse`:
- **Citations**: `createCitationSource` already returns `src.ID` set. Copy `src.ID` into `GenerateContentPart.ID`.
- **Web search results**: Call `generateID()` and set it on the `GenerateContentPart`.

### 4. Title field mapping in non-streaming citations

Currently, non-streaming citations map `src.Title` to `GenerateContentPart.Text`. This is pre-existing and unrelated to the ID fix; no change needed here.

## Risks / Trade-offs

- **Wire format addition**: Non-streaming responses will now include `"id"` on source content parts. This is additive (new field) and matches upstream, so it should not break existing consumers. `omitempty` ensures no empty `"id":""` appears for non-source parts.
- **Test coverage**: Existing tests for `convertResponse` and `emitWebSearchResult` will need updated golden values or new assertions for the ID field.
