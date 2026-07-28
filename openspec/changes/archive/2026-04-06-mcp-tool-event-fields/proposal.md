## Why

The orchestration layer's `translateToChunks` function drops `Dynamic`, `Title`, and `ProviderMetadata` fields when converting `StreamToolCall`, `StreamToolResult`, `StreamToolInputStart`, and `StreamToolError` into `UIMessageChunk` values. The provider layer (Anthropic) correctly populates all these fields on MCP tool events, and the serialization layer (`MarshalJSON`) already supports them -- but the translation step in between silently discards them.

This means MCP tool events (`tool-input-available`, `tool-output-available`, `tool-input-start`, `tool-output-error`) are missing `dynamic: true` and `providerMetadata` (with `anthropic.serverName` and `anthropic.type`) in the SSE wire format, diverging from the upstream TS SDK.

Ref: gh#90 (comment: "Additional finding: MCP tool events missing fields")

## What Changes

- Fix `translateToChunks` in `streamtext.go` to propagate `Dynamic`, `Title`, and `ProviderMetadata` from all tool-related `TextStreamPart` types to `UIMessageChunk`
- Affected cases: `StreamToolInputStart`, `StreamToolInputDelta`, `StreamToolCall`, `StreamToolResult`, `StreamToolError`

## Capabilities

### Modified Capabilities

- `mcp-server-tools`: MCP tool events now include `dynamic` and `providerMetadata` fields in the SSE wire format, matching upstream TS SDK behavior

## Impact

- **streamtext.go**: `translateToChunks` function -- add missing field assignments in 5 `case` branches (all tool chunk types except `tool-input-delta` gain `providerMetadata`; `tool-input-delta` correctly omits it per upstream schema)
- **chunk.go**: `MarshalJSON` -- add `setOptMeta` to `ChunkToolOutputAvailable`, `ChunkToolOutputError`, and `ChunkToolInputError` to match upstream schema
- **Wire format**: `tool-input-available`, `tool-input-start`, `tool-output-available`, `tool-output-error`, `tool-input-error` chunks gain `dynamic` and `providerMetadata` fields when present
- **Risk**: None -- fields use `setOptBool`/`setOptMeta` so they only appear in output when non-zero
