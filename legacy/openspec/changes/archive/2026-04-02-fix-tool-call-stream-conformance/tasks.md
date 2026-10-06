## 1. Skip empty input_json_delta

- [x] 1.1 In `anthropic/convert_stream.go`, add early return in the `input_json_delta` case when `delta.PartialJSON` is empty (still accumulate, but don't emit `PartToolInputDelta`)
- [x] 1.2 Add unit test in `anthropic/convert_stream_test.go` verifying empty `partial_json` does not produce a `PartToolInputDelta` stream part

## 2. Store and emit caller metadata on tool_use blocks

- [x] 2.1 Add `callerType` and `callerToolID` string fields to `blockState` in `anthropic/convert_stream.go`
- [x] 2.2 In the `tool_use` case of `content_block_start`, read `cb.Caller.Type` and `cb.Caller.ToolID` into blockState (only when `cb.Caller.Type` is non-empty)
- [x] 2.3 In the `tool_use` case of `content_block_stop`, build `ProviderMetadata` with `{"anthropic": {"caller": {"type": ..., "toolId": ...}}}` when `bs.callerType` is non-empty, and attach to the `PartToolCall` stream part
- [x] 2.4 Add unit tests for: tool_use with `caller.type=direct`, tool_use with `caller.type=code_execution_20250825` + `tool_id`, tool_use with no caller

## 3. Thread ProviderMetadata through orchestration to UI chunks

- [x] 3.1 In `streamtext.go` `translateToChunks`, add `ProviderMetadata: p.ProviderMetadata` to the `ChunkToolInputAvailable` UIMessageChunk for `StreamToolCall`
- [x] 3.2 In `streamtext.go` `translateToChunks`, add `ProviderMetadata: p.ProviderMetadata` to the `ChunkToolOutputAvailable` UIMessageChunk for `StreamToolResult`
- [x] 3.3 Add unit tests verifying ProviderMetadata passthrough for both `ChunkToolInputAvailable` and `ChunkToolOutputAvailable`
- [x] 3.4 In `chunk.go` `MarshalJSON`, add `setOptMeta(m, c.ProviderMetadata)` to the `ChunkToolOutputAvailable` case (discovered via conformance testing — metadata was silently dropped during serialization)
- [x] 3.5 In `streamtext.go` `executeTools`, propagate `ProviderMetadata` from `ToolCall` to `StreamToolResult` for locally-executed tools (discovered via conformance testing — `recorded/tool-call` fixture expected caller metadata on `tool-output-available`)

## 4. Verification

- [x] 4.1 Run `make test` to ensure all existing tests pass
- [x] 4.2 Run `make lint` to verify no lint issues
- [x] 4.3 Verify conformance tests pass against the `conformance-test-suite` branch fixtures (if locally available)
