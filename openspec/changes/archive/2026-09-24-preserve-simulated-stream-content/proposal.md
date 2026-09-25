## Why

`SimulateStreaming` still drops generated content fields in Go: text provider metadata never reaches `text-start`, and non-text conversion loses tool result flags/payloads, approval IDs, and source details. The registered `ai@7.0.107` middleware emits text metadata and forwards other generated parts intact; correcting this existing parity gap prevents downstream orchestration and UI streams from silently losing model output.

## What Changes

- Preserve provider metadata on nonempty generated text's `text-start`, while retaining existing text/reasoning segmentation and stream lifecycle.
- Preserve every applicable field of the nine supported `LanguageModelV4` generated content variants in the corresponding provider stream parts, including tool calls/results, approval requests, URL/document sources, files, reasoning files, and custom content. Retain Go's existing source/file data representations and request/response forwarding.
- Make simulated emission honor context cancellation even when a consumer stops reading a full channel; add exact stream-part regression tests and a provider-independent frontend-parsed UI integration scenario.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `builtin-middleware-simulate-streaming`: Specify full generated-content field preservation and cancellation behavior for the existing simulated language-model stream.

## Impact

Affects `middleware/simulate_streaming.go` and its tests, the existing middleware capability spec, and a Go testserver scenario plus Vitest integration test under `test/integration/`. `provider.GenerateContentPart` and `provider.StreamPart` already represent the supported fields; no public API, provider adapter, supported model-family, dependency, or upstream pin change is proposed. Validate the registered baseline in `test/conformance/upstream.yaml` (ai 7.0.107, commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`) without creating provider recordings or altering provider fixtures.
