## Why

`ReadUIMessageStream` currently buffers a `UIMessageChunk` channel until close, assembles one final `UIMessage`, sends that single value on a channel, and closes. That behavior is both misleading for a function named "Stream" and a parity gap with the registered upstream `ai@7.0.11` `readUIMessageStream`, which yields progressive message snapshots at upstream-defined write points as chunks update the message state.

The old helper also uses a channel for a single final value. Splitting it exposes the two actual use cases directly: progressive streaming snapshots and blocking final assembly.

While planning the split, the adjacent utility APIs exposed a broader ergonomics issue. The active pointer-options contract requires default calls such as `WriteUIMessageStream(w, result, nil)` and `ConvertToModelMessages(messages, nil)`. GitHub issue #93 identifies the same problem at the UI stream configuration layer: `UIMessageStreamOptions` uses `*bool` fields such as `SendReasoning`, `SendSources`, `SendFinish`, and `SendStart`, forcing callers to allocate temporary booleans or helper pointers. That fixed the older variadic-struct bug, but it left a noisy API that is inconsistent with the rest of the SDK's functional-option entry points. This change should replace the pointer-options utility convention with typed functional options so default calls omit configuration and multiple options compose intentionally.

## What Changes

- Remove `ReadUIMessageStream` instead of keeping a legacy compatibility wrapper. This is an intentional API cleanup and a breaking change for callers of the misleading helper.
- Add `StreamUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) <-chan UIMessage` as the progressive consumer API. It SHALL emit isolated `UIMessage` snapshots at the same state write points as upstream `ai@7.0.11` `readUIMessageStream` where those points are represented by Go `UIMessage` and `Part` types. It SHALL NOT emit a synthetic final snapshot merely because the input channel closes.
- Add `AssembleUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) (UIMessage, error)` as the blocking final-assembly API for callers that only want the completed message and an explicit error result.
- Replace pointer-style utility options with typed functional options for the affected helper entry points, incorporating issue #93 by removing the exported `UIMessageStreamOptions` pointer-bool configuration layer:
  - `ConvertToModelMessages(messages []UIMessage, opts ...ConvertOption) ([]provider.Message, error)`
  - `WriteUIMessageStream(w http.ResponseWriter, result *StreamTextResult, opts ...UIMessageStreamOption) error`
  - `(*StreamTextResult).ToUIMessageStream(opts ...UIMessageStreamOption) <-chan UIMessageChunk`
  - `StreamUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) <-chan UIMessage`
  - `AssembleUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) (UIMessage, error)`
- Remove the `utility-options-pointer` contract for these helpers and replace it with a `utility-functional-options` contract.
- Preserve `UIMessageChunk` wire format and SSE framing; this change affects only server-side consumption/assembly/conversion helper APIs.
- Add tests and docs that distinguish upstream write-point progressive snapshots from blocking assembly, use ergonomic default calls without `nil` or empty option structs, and compare behavior against the registered upstream baseline (`ai@7.0.11`).

## Capabilities

### New Capabilities
- `ui-message-stream-reader`: Progressive and blocking APIs for consuming `UIMessageChunk` streams into `UIMessage` snapshots or final assembled messages.
- `utility-functional-options`: Functional-option API style for utility helpers that previously required pointer option structs or empty config structs.

### Modified Capabilities
- `utility-options-pointer`: Remove the previous pointer-options requirements for `ConvertToModelMessages`, `WriteUIMessageStream`, and `ReadUIMessageStream`.

## Impact

- Public API: `ReadUIMessageStream` is removed from the root `aisdk` package; new `StreamUIMessage` and `AssembleUIMessage` functions are added; `ConvertToModelMessages`, `WriteUIMessageStream`, and `StreamTextResult.ToUIMessageStream` migrate to functional options. This resolves #93's `UIMessageStreamOptions` optional-boolean ergonomics by replacing `*bool` fields with scoped functional options.
- Implementation: `http.go:46-108`, `convert.go:14-24`, `streamtext.go:104-132`, and `stream.go:328-360` for public helper signatures, option types/builders, and shared UI message stream configuration; `stream.go:165-290` for extracting final assembly into reusable incremental state handling.
- Tests: existing pointer-option and `ReadUIMessageStream` tests migrate to ergonomic functional-option calls plus progressive snapshot coverage, blocking assembly coverage, lazy ID-generation behavior, partial tool-input JSON coverage, and `ChunkError`/invalid-order behavior for the new APIs.
- Docs: `README.md`, `doc.go`, `docs/guides/streaming-http.md`, `docs/concepts/wire-protocol.md`, `docs/getting-started/full-stack-chat.md`, `docs/best-practices/production.md`, and `docs/concepts/messages.md` update examples and terminology to use functional-option calls and the new reader helpers.
- Conformance/docs tooling: conformance config can still construct internal option values, but public API calls should use functional options.
- Parity: planning and implementation compare against `test/conformance/upstream.yaml` (`ai: 7.0.11`, `@ai-sdk/react: 4.0.12`) and upstream `packages/ai/src/ui-message-stream/read-ui-message-stream.ts` plus `packages/ai/src/ui/process-ui-message-stream.ts` at tag `ai@7.0.11`.
