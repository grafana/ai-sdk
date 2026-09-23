## 1. Establish a failing regression contract

- [ ] 1.1 Add a table-driven test in `middleware/simulate_streaming_test.go` comparing complete expected `provider.StreamPart` sequences for each of the nine supported generate variants; include text/reasoning metadata, empty text and IDs, tool-call input, tool-result JSON/error/preliminary (true and explicit false), approval ID, both source types and fields, file/reasoning-file bytes/base64/URL/empty bytes, custom metadata, and mixed ordering. Assert the upstream-shaped source wire JSON where nesting differs.
- [ ] 1.2 Add cancellation tests for a stream blocked with more than the 64-part channel buffer and cancellation during multipart emission; cancel before draining a full channel, then assert bounded closure and no `PartFinish` or full generated suffix. Allow a race-dependent post-cancel prefix rather than asserting an exact count. Run `go test ./middleware -run TestSimulateStreaming` and confirm the new fidelity/cancellation assertions fail on the old implementation.
- [ ] 1.3 Add a deterministic Go-generated-content scenario in `test/integration/testserver/` using `middleware.SimulateStreaming()` with text provider metadata and a document source; call `ToUIMessageStream(aisdk.WithUIMessageStreamSources(true))` to emit source chunks and add a matching Vitest test asserting schema-parsed SSE chunk fields and `readUIMessageStream` assembly. Confirm the scenario exposes the existing omission before fixing it. Do not author provider recorded/upstream inputs.

## 2. Preserve generated parts and unblock cancellation

- [ ] 2.1 Update `middleware/simulate_streaming.go` to put generated text's `ProviderMetadata` on `PartTextStart` and retain the existing text/reasoning lifecycle, IDs, response metadata and finish data.
- [ ] 2.2 Expand `contentPartToStreamPart` only for supported v4 generated parts: tool result `Result`/`IsError`/`Preliminary`/`Dynamic`, approval ID and all nested source fields/metadata; retain tool-call, file/reasoning-file and custom fields with current Go data representations. Pass the exact-stream-part and source-wire tests.
- [ ] 2.3 Make every emitting-goroutine channel send (including lifecycle, multipart content, default content and finish) select on the call context, exiting and closing the stream when canceled. Pass both blocked-consumer tests without a goroutine leak.

## 3. Validate registered parity and frontend behavior

- [ ] 3.1 Run focused Go middleware and Go testserver tests, `mise run test-integration` for the schema-parsed frontend scenario, and `mise run parity-check` against the registered baseline; resolve failures caused by this change.
- [ ] 3.2 Check whether any existing provider-independent UI fixture expectation changes and regenerate only affected expectations if so; preserve provider fixture provenance and document any remaining provider-boundary coverage gap. Do not change upstream pins or claim that UI assertions cover raw fields they cannot expose.
