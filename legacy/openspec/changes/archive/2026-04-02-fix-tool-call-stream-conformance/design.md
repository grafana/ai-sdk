## Context

The Go Anthropic provider's stream adapter (`anthropic/convert_stream.go`) has two conformance gaps versus the upstream TypeScript SDK. The conformance test suite on the `conformance-test-suite` branch catches both. The orchestration layer (`streamtext.go`) also drops `ProviderMetadata` when mapping tool stream parts to UI chunks, preventing provider metadata from reaching the wire even if the provider emits it.

Upstream reference:
- Empty delta skip: `packages/anthropic/src/anthropic-messages-language-model.ts:1978-1986`
- Caller metadata: `packages/anthropic/src/anthropic-messages-language-model.ts:1484-1497, 1888-1904`
- UI passthrough: `packages/ai/src/generate-text/stream-text.ts:2425-2438, 2453-2470`

## Goals / Non-Goals

**Goals:**
- Match upstream behavior for empty `input_json_delta` events (skip, don't emit)
- Carry `anthropic.caller` metadata from `content_block_start` through to `PartToolCall`
- Thread `ProviderMetadata` from `StreamToolCall`/`StreamToolResult` to `ChunkToolInputAvailable`/`ChunkToolOutputAvailable` UI chunks
- Pass conformance test fixtures `tool-call-no-args` and `tool-call`

**Non-Goals:**
- Adding caller metadata to non-streaming (`convertResponse`) path (separate follow-up if needed)
- Adding caller to `tool-input-start` parts (upstream doesn't include it there either)
- Changes to MCP tool handling (already has its own providerMetadata path)

## Decisions

### 1. Skip empty `input_json_delta` in the stream adapter

**Decision**: Add an early return in the `input_json_delta` case when `delta.PartialJSON` is empty.

**Rationale**: Direct match of upstream behavior. The upstream comment references code execution tool compatibility. The empty delta still gets accumulated (harmless empty string concat), but no stream part is emitted.

**Alternative considered**: Accumulate but don't emit — functionally identical, chosen the simpler "skip entirely" approach matching upstream.

### 2. Store caller in blockState, attach on PartToolCall

**Decision**: Add a `callerType` and `callerToolID` field to `blockState`. Populate from `cb.Caller` during `content_block_start` for `tool_use` blocks. When the block stops and `PartToolCall` is emitted, build `ProviderMetadata` with `{"anthropic": {"caller": {"type": ..., "toolId": ...}}}` if caller type is non-empty.

**Rationale**: Matches the upstream pattern where `contentBlock.caller` is stored during `content_block_start` and attached to the `tool-call` part at `content_block_stop`. Using separate string fields rather than storing the full SDK union type keeps the blockState simple and avoids coupling to SDK internals.

**Alternative considered**: Store the raw `BetaRawContentBlockStartEventContentBlockUnionCaller` struct — rejected because it couples blockState to the Anthropic SDK's union type and we only need two string fields.

### 3. Thread ProviderMetadata in translateToChunks

**Decision**: In `streamtext.go`'s `translateToChunks` function, add `ProviderMetadata` to the `UIMessageChunk` for both `StreamToolCall` → `ChunkToolInputAvailable` and `StreamToolResult` → `ChunkToolOutputAvailable`.

**Rationale**: The upstream `stream-text.ts` explicitly passes `providerMetadata` through on both `tool-input-available` and `tool-output-available` chunks. The Go types already have the field; it's just not being set in the mapping function.

### 4. Add `setOptMeta` for `ChunkToolOutputAvailable` in MarshalJSON

**Decision**: In `chunk.go`'s custom `MarshalJSON`, add `setOptMeta(m, c.ProviderMetadata)` to the `ChunkToolOutputAvailable` case.

**Rationale**: The custom marshaler only serializes explicitly listed fields for each chunk type. `ChunkToolOutputAvailable` was missing the `providerMetadata` call, so the field was silently dropped even when set on the struct. Discovered during conformance testing — the struct-level fix alone wasn't sufficient because the custom marshaler gates what reaches the wire.

### 5. Propagate ProviderMetadata from ToolCall to local tool result

**Decision**: In `streamtext.go`'s `executeTools`, add `ProviderMetadata: tc.ProviderMetadata` to the `StreamToolResult` emitted after a locally-executed tool completes.

**Rationale**: When a tool is executed locally (not provider-executed), the `StreamToolResult` needs to carry forward the `ProviderMetadata` from the originating `ToolCall`. The upstream SDK propagates caller metadata through to `tool-output-available` for locally-executed tools. Without this, the conformance fixture `recorded/tool-call` fails because the `tool-output-available` chunk is missing the `anthropic.caller` metadata.

## Risks / Trade-offs

- **[Low] Wire format change**: Adding `providerMetadata` to tool chunks changes what's sent over SSE. This is additive (new field, previously absent) and matches upstream, so `@ai-sdk/react` consumers already handle it. → No migration needed.
- **[Low] Anthropic SDK coupling**: We read `cb.Caller.Type` and `cb.Caller.ToolID` from the union type. If the Anthropic Go SDK changes this struct, we'll need to update. → Acceptable; we already depend on the SDK's event types throughout the adapter.
