## Why

The conformance test suite (branch `conformance-test-suite`) found two bugs in the Go Anthropic provider's tool call stream handling that cause UIMessageChunk output to diverge from the upstream TypeScript SDK. These must be fixed to maintain wire compatibility with `@ai-sdk/react` consumers.

## What Changes

- **Skip empty `tool-input-delta` chunks**: When Anthropic sends `input_json_delta` with empty `partial_json`, the Go provider currently emits a `tool-input-delta` chunk with empty delta. The upstream TS SDK skips these. This shifts subsequent chunk indices and breaks conformance.
- **Pass `providerMetadata` with `anthropic.caller` on tool call parts**: The upstream TS SDK reads the `caller` field from Anthropic's `content_block_start` event and attaches it as `providerMetadata: { anthropic: { caller: { type, toolId } } }` on `tool-call` stream parts (which become `tool-input-available` UI chunks). The Go SDK currently omits this metadata entirely.
- **Thread `providerMetadata` through to UI chunks**: The orchestration layer (`streamtext.go`) drops `ProviderMetadata` when mapping `StreamToolCall` → `ChunkToolInputAvailable` and `StreamToolResult` → `ChunkToolOutputAvailable`. This needs to be threaded through so provider metadata reaches the wire.
- **Serialize `providerMetadata` on `tool-output-available` chunks**: The custom `MarshalJSON` in `chunk.go` omits `providerMetadata` from `ChunkToolOutputAvailable`, so even if the struct field is set the metadata is silently dropped on the wire.
- **Propagate `providerMetadata` from tool call to local tool result**: When `executeTools` runs a locally-defined tool, the resulting `StreamToolResult` must carry forward the `ProviderMetadata` from the originating `ToolCall` so it reaches the UI chunk.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `server-tools`: The `caller` metadata passthrough is part of how tool events are surfaced on the wire. The requirement change is: tool-call stream parts must carry `providerMetadata` from the provider through to the UI chunk layer.

## Impact

- `anthropic/convert_stream.go` — skip empty input_json_delta, store caller in blockState, attach to PartToolCall
- `streamtext.go` — thread ProviderMetadata in translateToChunks and propagate in executeTools
- `chunk.go` — add `setOptMeta` for `ChunkToolOutputAvailable` in MarshalJSON
- `anthropic/convert_stream_test.go` — new tests for empty delta skip and caller metadata
- `chunk_test.go` — verify providerMetadata passthrough for both UI chunk types
- Conformance test fixtures (`tool-call-no-args`, `tool-call`) should pass after fix
