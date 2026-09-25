## Context

The issue is still reproducible in the current `middleware/simulate_streaming.go:40-125`. Generated text is split without `ProviderMetadata` on `PartTextStart`; `contentPartToStreamPart` copies only selected fields and drops tool result `Result`/`IsError`/`Preliminary`, approval `ApprovalID`, and source `ID`/`Title`/document `MediaType`/`Filename`. Existing partial tests (`middleware/simulate_streaming_test.go`) do not expose all losses. The existing spec already promises non-text pass-through, so the delta makes its field preservation explicit.

The registered upstream reference is `ai@7.0.107`, commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d` (`test/conformance/upstream.yaml`). At that exact commit `packages/ai/src/middleware/simulate-streaming-middleware.ts` puts generated text metadata on `text-start`, emits reasoning start/delta/end, and enqueues each other content part unchanged; its test asserts generated text metadata. The corresponding provider v4 content union has nine variants, all represented by `provider.GenerateContentPart` (`provider/language_model.go:114-164`) and `provider.StreamPart` (`provider/stream_part.go:13-43,147-209`). The Go stream's file data and nested source structures are deliberate representations of the same wire fields; the upstream synchronous `ReadableStream.start` cannot be copied into Go's buffered channel goroutine. `test/conformance/PARITY.md` classifies this as middleware/core-orchestration, provider-contract and UI/SSE evidence, not a provider recording.

## Goals / Non-Goals

**Goals:**
- Lossless projection of *represented, applicable* fields on all nine supported generated content variants, retaining exact stream-part order and existing request/response/finish semantics.
- Keep Go context cancellation effective even if a consumer abandons a buffered stream.
- Establish unit-level field-complete proof and frontend-interoperable UI proof from a synthetic, provider-independent Go model.

**Non-Goals:**
- New provider/model APIs, other unsupported model families, new generated content variants, or general provider data conversion changes.
- Altering upstream pins or hand-writing provider recorded/upstream fixtures; claiming that frontend UI chunks expose every raw provider field.
- Changing response framing or the behavior of the other middleware implementations.

## Decisions

### 1. Expand the existing projection, without changing the provider contract

Update `middleware/simulate_streaming.go` in place. Copy `ProviderMetadata` onto `PartTextStart` only for nonempty text, as upstream does; keep `PartReasoningStart` metadata and the current ID progression (increment for nonempty text and every reasoning part, not for default/pass-through parts). Preserve stream-start warnings, response metadata (including absent response), finish reason/usage/provider metadata, and returned request/response headers.

For the default branch, keep `contentPartToStreamPart` as the Go projection: `ContentToolCall` retains `ToolCallID`, `ToolName`, stringified raw-JSON `Input`, `ProviderExecuted`, `Dynamic` and metadata; `ContentToolResult` retains `ToolCallID`, `ToolName`, `Result` as raw JSON, `IsError`, `Preliminary` (including explicit `false`), `Dynamic` and metadata; `ContentToolApprovalRequest` retains `ApprovalID`, `ToolCallID` and metadata; `ContentCustom` retains `Kind` and metadata. Preserve file/reasoning-file `Data` (raw bytes including empty bytes, base64 or URL without unnecessary conversion), `MediaType` and metadata. For `ContentSource`, populate the complete `StreamPart.Source` (`SourceInfo.SourceType`, `ID`, `URL`, `Title`, `MediaType`, `Filename`, `ProviderMetadata`); `provider.StreamPart.MarshalJSON` already flattens the nested source into the upstream wire form (`provider/stream_part.go:255-270`). Do not invent a new representation or change `provider/stream_part.go`. Align projection of every applicable field with upstream variant definitions rather than copying unrelated flat-union fields into each event.

**Alternative:** JSON round-tripping generated parts into stream parts would also share fields, but source and file union encoding plus stringified tool-call input differ internally; explicit typed mapping is smaller and already established here.

### 2. Make all synthetic sends context-aware

The emitting goroutine currently uses unconditional `ch <-` for the lifecycle and every content event. Use a small local send helper that selects between `ctx.Done()` and sending, returning false when canceled; stop the goroutine (and close its channel via existing defer) when a send cannot complete. Cover stream-start, response-metadata, text/reasoning subparts, default branch and finish. Preserve the channel buffer and part ordering when context is active. This cancellation path is a Go adaptation to the upstream stream controller, not a change to the provider surface.

**Alternative:** Enlarging the buffer or relying on consumers to drain still allows blocked goroutines when consumers abandon streams; a context-aware send is bounded and testable. Cancellation after an event becomes sendable is a race, so tests assert eventual closure and no blocked producer, not an exact post-cancel prefix.

### 3. Regression evidence at the correct layers

First write failing table-driven `middleware/simulate_streaming_test.go` cases asserting **complete** expected `[]provider.StreamPart` for each of the nine `GenerateContentType` variants (full lifecycle included or an explicit shared wrapper). Include nonempty/empty text and metadata, reasoning, tool-call raw input, tool result JSON with `IsError` and both `Preliminary` values, approvals, URL and document source with IDs/titles/media/filename/provider metadata, bytes/base64/URL file and reasoning-file including empty bytes, custom metadata and a mixed-part ordering/ID case. Compare direct structs (including pointers) and verify source wire JSON for flattening; use the preexisting tests for response metadata/DoGenerate passthrough. Use a blocking-consumer test with **more than 64** parts: wait until the channel fills without consuming, cancel while the producer is blocked, then drain with a deadline. Assert bounded closure **and** that the stream stops before its complete generated suffix and `PartFinish`; merely closing after draining would also pass with the old producer. Do not require an exact post-cancel prefix count. Also cover cancellation during a multipart emission so every send path is reachable. Keep synthetic input only in these focused tests.

Add one deterministic Go `DoGenerate` test model and `middleware.WrapLanguageModel(model, middleware.SimulateStreaming())` scenario in `test/integration/testserver/` routed through `aisdk.StreamText`/`ToUIMessageStream(aisdk.WithUIMessageStreamSources(true))`, with generated text metadata and document source details (plus a custom part if the UI path exposes it), and a matching `test/integration/*.test.ts`. Sources are disabled by default in `stream.go`, so explicitly enable them as in `test/integration/testserver/scenario_source_document.go`. Parse SSE via `parseJsonEventStream` plus `uiMessageChunkSchema`, assert emitted chunk fields, and assemble with `readUIMessageStream` to assert metadata, text and source fields, following `sse-message-assembly.test.ts` and `scenario_source_document.go`. This proves frontend interop for fields surfaced by UI, while the table covers provider-part fields that UI projection may omit. Add a provider-independent `test/conformance/ui/` fixture only if existing UI fixture input replay is genuinely affected; do not invent a provider `recorded/` or `upstream/` input.

## Risks / Trade-offs

- **Raw vs UI representation:** frontend chunks do not necessarily expose all provider fields → assert full raw parts in middleware unit tests, UI-visible fields in schema-parsed integration, and explain the distinction in coverage evidence.
- **Empty data vs absent data:** a zero-length byte slice is valid file data → preserve `Bytes != nil` precedence over empty base64/URL and test it.
- **Cancellation timing:** a buffered send can complete concurrently with cancellation → assert bounded termination and early truncation (no `PartFinish` or full generated suffix), not an exact canceled-event count; avoid goroutine leaks in test cleanup.
- **Existing consumers might observe newly retained metadata/fields:** additive correction to existing wire content, no migration or endpoint changes; roll back the middleware projection if behavior regresses, retaining the tests as reproduction.

## Migration Plan

No schema or public API migration. Land projection, targeted tests and integration scenario together; run `go test ./middleware`, `mise run test-integration`, `mise run parity-check`, and `openspec validate preserve-simulated-stream-content --strict`. Existing fixture snapshots only need regeneration if the actual UI output of a covered scenario changes.

## Open Questions

None requiring a product/API decision. `DataContent.Text`/`Reference` are not registered v4 generated file union options; do not silently add support under this change. Document any separate provider-boundary coverage gap if integration cannot establish a particular UI projection.
