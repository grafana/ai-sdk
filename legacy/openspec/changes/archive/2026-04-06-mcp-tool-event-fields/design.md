## Context

The three-layer event pipeline: `provider.StreamPart` -> `TextStreamPart` (e.g., `StreamToolCall`) -> `UIMessageChunk` (SSE wire format). The provider layer correctly populates `Dynamic`, `Title`, and `ProviderMetadata` on MCP tool events. The `UIMessageChunk` struct has all these fields, and `MarshalJSON` serializes them via `setOptBool`/`setOpt`/`setOptMeta` (they only appear in output when non-zero). The gap is in the middle: `translateToChunks()` in `streamtext.go` doesn't copy these fields from `TextStreamPart` to `UIMessageChunk`.

## Goals / Non-Goals

**Goals:**

- Propagate `Dynamic`, `Title`, and `ProviderMetadata` from all tool `TextStreamPart` types through `translateToChunks` to `UIMessageChunk`
- Match upstream TS SDK wire format for MCP tool events

**Non-Goals:**

- Changing the `UIMessageChunk` struct (it already has all needed fields)
- Changing any provider-layer code (it already populates the fields correctly)
- Adding new chunk types or fields

## Decisions

### 1. Fix all tool chunk translations, not just `tool-input-available`

**Decision**: Propagate the missing fields on all five tool-related cases in `translateToChunks`: `StreamToolInputStart`, `StreamToolInputDelta`, `StreamToolCall`, `StreamToolResult`, and `StreamToolError`.

**Rationale**: The issue manifests most visibly on MCP `tool-input-available`, but the same fields are dropped on all tool chunks. Fixing them all ensures consistency and prevents future conformance gaps. Since `setOptBool`/`setOpt`/`setOptMeta` only emit non-zero values, adding these fields to non-MCP tool chunks has no effect on wire output.

### 2. Add `providerMetadata` serialization to output/error chunks

**Decision**: Add `setOptMeta` to `MarshalJSON` for `ChunkToolOutputAvailable`, `ChunkToolOutputError`, and `ChunkToolInputError`. The upstream schema includes `providerMetadata` on all three.

**Rationale**: Upstream `ui-message-chunks.ts` defines `providerMetadata` as optional on these chunk types. Without it, MCP tool output events would be missing metadata on the wire. Note: upstream does NOT include `providerMetadata` on `tool-input-delta` or `title` on `tool-output-*` -- those are correctly omitted.

## Risks / Trade-offs

**[Risk: None]** The fix is mechanical -- adding field assignments that were accidentally omitted. The types, serialization, and provider layer all already support these fields.
