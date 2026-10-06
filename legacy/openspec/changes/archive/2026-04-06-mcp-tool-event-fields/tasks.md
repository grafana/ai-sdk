## 1. Fix translateToChunks field propagation

- [x] 1.1 Add `Dynamic`, `Title`, `ProviderMetadata` to `StreamToolInputStart` -> `ChunkToolInputStart` translation (streamtext.go:156)
- [x] 1.2 ~~Add `ProviderMetadata` to `StreamToolInputDelta`~~ Upstream schema excludes `providerMetadata` on `tool-input-delta` -- omitted
- [x] 1.3 Add `Dynamic`, `Title`, `ProviderMetadata` to `StreamToolCall` -> `ChunkToolInputAvailable` translation (streamtext.go:162)
- [x] 1.4 Add `Dynamic`, `Title`, `ProviderMetadata` to `StreamToolResult` -> `ChunkToolOutputAvailable` translation (streamtext.go:164)
- [x] 1.5 Add `Dynamic`, `Title`, `ProviderMetadata` to `StreamToolError` -> `ChunkToolOutputError` translation (streamtext.go:170)

## 2. Tests

- [x] 2.1 Add test for `translateToChunks` verifying `Dynamic`, `Title`, and `ProviderMetadata` propagation on MCP tool input available chunk
- [x] 2.2 Add test for `translateToChunks` verifying fields propagate on tool output available and tool error chunks
- [x] 2.3 Add test verifying non-MCP tool chunks still omit these fields (zero values not emitted)

## 3. MarshalJSON upstream alignment

- [x] 3.1 Add `setOptMeta` to `ChunkToolOutputAvailable` MarshalJSON (chunk.go)
- [x] 3.2 Add `setOptMeta` to `ChunkToolOutputError` MarshalJSON (chunk.go)
- [x] 3.3 Add `setOptMeta` to `ChunkToolInputError` MarshalJSON (chunk.go)

## 4. Verification

- [x] 4.1 Run `make test` -- all tests pass
- [x] 4.2 Run `make vet` and `make lint` -- no issues
