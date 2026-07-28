## 1. Upstream and API Preparation

- [x] 1.1 Reconfirm the registered upstream baseline in `test/conformance/upstream.yaml` and compare reader behavior against `ai@7.0.11` `packages/ai/src/ui-message-stream/read-ui-message-stream.ts` and `packages/ai/src/ui/process-ui-message-stream.ts`, especially write points, `ChunkError` handling, final-close behavior, and partial tool input parsing.
- [x] 1.2 Remove the public `ReadUIMessageStream` helper, `ReadStreamOption`, and their godoc from `http.go`.
- [x] 1.3 Add public godoc declarations in `http.go` for `StreamUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) <-chan UIMessage` and `AssembleUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) (UIMessage, error)`.
- [x] 1.4 Define a sealed/typed `UIMessageReaderOption` functional-option interface and `WithUIMessageReaderGenerateID(fn func() string)` option.
- [x] 1.5 Search repository callers/tests/docs for `ReadUIMessageStream` and migrate them to `StreamUIMessage` or `AssembleUIMessage` as appropriate.

## 2. Utility Functional Options Migration

- [x] 2.1 Replace exported `ConvertOptions` pointer configuration with a typed `ConvertOption` functional-option interface and `WithIgnoreIncompleteToolCalls()` option.
- [x] 2.2 Change `ConvertToModelMessages` to `func ConvertToModelMessages(messages []UIMessage, opts ...ConvertOption) ([]provider.Message, error)` and migrate all callers from `nil`/`&ConvertOptions{...}` to zero or more functional options.
- [x] 2.3 Replace exported `UIMessageStreamOptions` pointer/value configuration for public APIs with a typed `UIMessageStreamOption` functional-option interface, resolving #93's pointer-bool ergonomics concern.
- [x] 2.4 Add functional options covering the existing UI message stream fields: original messages, generated message ID, finish callback, message metadata callback, send reasoning, send sources, send finish, send start, and error-to-text callback.
- [x] 2.5 Change `WriteUIMessageStream` to `func WriteUIMessageStream(w http.ResponseWriter, result *StreamTextResult, opts ...UIMessageStreamOption) error` and migrate all callers from `nil`/`&UIMessageStreamOptions{...}` to zero or more functional options.
- [x] 2.6 Change `(*StreamTextResult).ToUIMessageStream` to `func (r *StreamTextResult) ToUIMessageStream(opts ...UIMessageStreamOption) <-chan UIMessageChunk` and migrate all callers from `UIMessageStreamOptions{}`/struct literals to zero or more functional options.
- [x] 2.7 Keep unexported internal config structs if useful, but remove or stop exposing public pointer option structs so there is one supported ergonomic configuration style.
- [x] 2.8 Update lower-level internal helpers such as `translateToChunks`, `filterChunks`, and conformance config adapters to use the internal config form without re-exposing pointer-style public APIs.
- [x] 2.9 Preserve current `OriginalMessages != nil` sentinel behavior with an internal presence flag so `WithUIMessageStreamOriginalMessages()` and `WithUIMessageStreamOriginalMessages(messages...)` with a nil slice mean explicitly present empty history, while omitting the option means absent history.
- [x] 2.10 Make functional option builders ignore accidental nil option values safely while docs and repository call sites use no option arguments for defaults.

## 3. Shared Reader State Implementation

- [x] 3.1 Extract the current `assembleResponseMessage` logic into a reusable incremental UI message reader state that can apply one `UIMessageChunk` at a time.
- [x] 3.2 Track active text and reasoning parts by chunk ID, partial tool calls by tool call ID, and data parts by data name/ID so progressive updates mutate existing parts instead of appending duplicates.
- [x] 3.3 Preserve existing final assembly behavior for valid streams containing text, reasoning, files, sources, tool approval/output states, metadata, step starts, non-transient data, transient data exclusion, and data reconciliation by ID.
- [x] 3.4 Return package-prefixed errors from the state updater for invalid stateful sequences such as text delta before text start or tool output before a matching tool input.
- [x] 3.5 Treat `ChunkError` separately from malformed state. `StreamUIMessage` skips the error chunk, emits no snapshot for it, and continues consuming; `AssembleUIMessage` records/returns a non-nil error.
- [x] 3.6 Generate fallback message IDs lazily: prefer a start chunk ID observed before the first emitted snapshot, call the custom/default generator exactly once only when the first snapshot needs an ID, and let `AssembleUIMessage` delay fallback generation until the final return when possible.

## 4. Progressive Tool Input and Snapshot Semantics

- [x] 4.1 Add state handling for progressive tool-input-start and tool-input-delta chunks, including static versus dynamic tool parts and partial input updates represented by existing Go `Part` types.
- [x] 4.2 Maintain accumulated input text per tool call and port upstream `ai@7.0.11` partial JSON behavior: parse complete JSON, then try repaired JSON using a Go equivalent of upstream `fixJson`, and otherwise treat input as unavailable for that snapshot.
- [x] 4.3 Ensure tool-input-delta snapshots never contain invalid `json.RawMessage`; successful parse/repair results are marshaled back to valid JSON bytes, and failed parse results leave `Input` nil/omitted while preserving `input-streaming` state.
- [x] 4.4 Emit progressive snapshots only at upstream `ai@7.0.11` write points represented in Go. Do not emit a synthetic snapshot on channel close; empty streams and streams with only non-writing chunks close without yielding a message.
- [x] 4.5 Implement deep-copy snapshot helpers for `UIMessage`, all current `Part` implementations, raw JSON payloads, provider metadata maps, and nested approval values before sending each progressive snapshot.

## 5. Progressive and Blocking APIs

- [x] 5.1 Implement `StreamUIMessage` to consume the input channel in order, apply the shared state updater, skip `ChunkError` chunks without closing, close on invalid state transitions, and emit snapshots at upstream write points only.
- [x] 5.2 Implement `AssembleUIMessage` to drain the input channel, apply the same state transitions, return the final message directly, and return non-nil errors for stream error chunks or invalid chunk ordering.
- [x] 5.3 Ensure `AssembleUIMessage` returns an assistant message with a generated ID and no parts for an empty stream, and returns the final state even when a stream has no progressive write-point snapshots.
- [x] 5.4 Ensure no exported `ReadUIMessageStream`, `ReadStreamOption`, `ConvertOptions`, or `UIMessageStreamOptions` public configuration symbols remain after the replacement unless a documented compatibility decision is explicitly added to this change.

## 6. Tests

- [x] 6.1 Replace existing `ReadUIMessageStream` tests with coverage for `StreamUIMessage` and `AssembleUIMessage`.
- [x] 6.2 Add option API tests or compile-oriented examples proving default calls omit config: `ConvertToModelMessages(messages)`, `WriteUIMessageStream(w, result)`, `result.ToUIMessageStream()`, `StreamUIMessage(chunks)`, and `AssembleUIMessage(chunks)`.
- [x] 6.3 Add functional-option tests proving configured calls work for conversion, UI message stream output flags/callbacks, reader ID generation, nil option values being ignored safely, and repeated scalar/callback options using last value.
- [x] 6.4 Add `http_test.go` coverage for progressive text and reasoning snapshots, including accumulated deltas, completion behavior, and exact upstream write-point expectations.
- [x] 6.5 Add progressive tool lifecycle tests covering tool input streaming, valid partial input, repaired partial input, failed partial input with nil/omitted `Input`, input available, approval request/response, output available, output denied, output error, and dynamic tool parts.
- [x] 6.6 Add tests proving progressive tool-input snapshots always marshal successfully and never contain invalid `json.RawMessage`, even when accumulated partial input cannot be parsed or repaired.
- [x] 6.7 Add tests for non-text parts and metadata: files, source URL/document parts, step starts, non-transient data reconciliation, transient data exclusion/no snapshot, start metadata, message-metadata chunks, finish metadata, and non-writing finish/step behavior.
- [x] 6.8 Add tests proving each progressive snapshot is isolated by mutating an earlier snapshot and asserting later snapshots and final assembly are unaffected.
- [x] 6.9 Add `StreamUIMessage` tests for `ChunkError` parity: no snapshot for the error chunk and later valid chunks continue to emit snapshots.
- [x] 6.10 Add `StreamUIMessage` tests for empty streams and streams containing only non-writing chunks, asserting the output channel closes without emitted messages.
- [x] 6.11 Add `AssembleUIMessage` tests for final-message equality with the last progressive snapshot only when no later non-writing state mutations occur, divergence when a writing chunk is followed by a non-writing state-mutating `ChunkStartStep` that appears only in blocking final assembly, final state for no-snapshot streams, empty-stream generated ID behavior, custom reader ID option, `ChunkError`, and invalid chunk ordering errors.
- [x] 6.12 Add lazy ID-generation tests proving a custom reader generator is not called when a start chunk supplies the ID before the first emitted snapshot and is called exactly once when the first snapshot or final assembly needs a fallback ID.
- [x] 6.13 Add repository-level checks or focused assertions that old public call shapes using pointer option structs, `UIMessageStreamOptions{}`, and `ReadUIMessageStream` no longer appear in user-facing docs/examples after migration; default examples should omit options instead of passing `nil`.
- [x] 6.14 Add tests for `WithUIMessageStreamOriginalMessages()` and nil-slice expansion preserving the current explicitly-present-empty-history behavior for continuation IDs and finish callbacks.

## 7. Documentation and Parity Notes

- [x] 7.1 Update `README.md`, `doc.go`, `docs/getting-started/full-stack-chat.md`, and `docs/best-practices/production.md` to use `WriteUIMessageStream(w, result)` without `nil`.
- [x] 7.2 Update `docs/guides/streaming-http.md` to show `StreamUIMessage` for progressive updates, `AssembleUIMessage` for blocking final assembly, and functional `ToUIMessageStream`/`WriteUIMessageStream` options.
- [x] 7.3 Update `docs/concepts/wire-protocol.md` to distinguish chunk streaming, progressive message snapshots, and blocking assembly while stating that the chunk wire format is unchanged.
- [x] 7.4 Update `docs/concepts/messages.md` to use `ConvertToModelMessages(messages)` and functional conversion options.
- [x] 7.5 Remove `ReadUIMessageStream`, pointer-options, and empty-struct option examples from user-facing docs rather than documenting them as deprecated.
- [x] 7.6 Review `test/conformance/PARITY.md` and related parity documentation to classify progressive UI message reader coverage; update it if this new parity surface is not already represented.
- [x] 7.7 Note scoped intentional differences from upstream `ai@7.0.11` reader options, especially the absence of upstream `message`, `onError`, and `terminateOnError` options, while documenting that `ChunkError` streaming behavior follows upstream's default `terminateOnError=false`.

## 8. Validation

- [x] 8.1 Run `gofmt` on changed Go files.
- [x] 8.2 Run focused root package tests for utility option and reader behavior, such as `go test -run 'Test.*Options|Test.*UIMessageStream|Test.*StreamUIMessage|Test.*AssembleUIMessage|Test.*ConvertToModelMessages' ./...`.
- [x] 8.3 Run `go test ./...` for the root module.
- [x] 8.4 Run `mise run validate-parity-baseline` and any focused parity/conformance checks warranted by the implementation's final coverage changes.
- [x] 8.5 Run `openspec validate split-ui-message-stream-reader --type change --strict --json` before applying/archive handoff.
