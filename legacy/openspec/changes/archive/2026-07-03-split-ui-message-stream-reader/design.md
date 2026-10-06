## Context

`ReadUIMessageStream` in `http.go:85-108` currently consumes all `UIMessageChunk` values, stores them in a slice, calls `assembleResponseMessage` from `stream.go:165-290`, sends one `UIMessage` on a buffered channel, and closes. The only reader option today is `ReadStreamOption.GenerateID` (`http.go:80-83`). Existing tests in `http_test.go:132-222` assert final assembly only.

The registered upstream baseline is `ai@7.0.11` and `@ai-sdk/react@4.0.12` in `test/conformance/upstream.yaml`. At that baseline, `packages/ai/src/ui-message-stream/read-ui-message-stream.ts` returns an async iterable stream and enqueues `structuredClone(state.message)` from `processUIMessageStream` after upstream-defined write points. Upstream does not enqueue an extra message when the input closes, and it does not keep a separate blocking legacy helper named like the progressive reader.

This change closes the progressive-consumption gap and cleans up the misleading public API without changing `UIMessageChunk` serialization or SSE framing.

A related ergonomics issue spans the utility helpers around this code path. `ConvertToModelMessages` and `WriteUIMessageStream` currently take pointer option structs because an earlier `fix-variadic-options-api` change replaced variadic option structs that silently ignored extra values. That pointer contract made the zero-or-one shape compile-time explicit, but default calls now require noisy `nil` arguments. `StreamTextResult.ToUIMessageStream` has the same ergonomic problem in a different shape: it requires `UIMessageStreamOptions{}` for defaults. GitHub issue #93 calls out an additional layer of this problem: the `UIMessageStreamOptions` optional booleans use `*bool` to model TypeScript `boolean | undefined`, making simple calls like enabling sources or hiding reasoning require temporary variables. The rest of the SDK's main entry points (`StreamText`, `GenerateText`, providers, registries, and output builders) already use typed functional options for optional behavior.

## Goals / Non-Goals

**Goals:**

- Replace `ReadUIMessageStream` with explicit progressive and blocking APIs.
- Introduce a progressive `StreamUIMessage` helper that yields `UIMessage` snapshots while chunks arrive.
- Introduce a blocking `AssembleUIMessage` helper that returns the final assembled message directly with an error result.
- Replace the pointer-options utility convention with typed functional options so default calls can omit config and multiple options compose intentionally.
- Migrate adjacent utility entry points to ergonomic calls: `ConvertToModelMessages(messages)`, `WriteUIMessageStream(w, result)`, and `result.ToUIMessageStream()`.
- Preserve valid-stream final assembly semantics for text, reasoning, files, sources, tool states, metadata, and data reconciliation while adding progressive state updates.
- Compare the implementation against upstream `ai@7.0.11`, not upstream main or the local checkout's current package version.

**Non-Goals:**

- No `UIMessageChunk` schema, SSE framing, or frontend hook wire-format changes.
- No upstream baseline upgrade.
- No broad expansion of reader options to cover upstream `message`, `onError`, or `terminateOnError`; those can be proposed separately if needed.
- No replacement of producer-side `CreateUIMessageStream` or `WriteTextStream` APIs.
- No legacy compatibility wrapper for the old `ReadUIMessageStream` name.
- No reintroduction of variadic option structs such as `opts ...ReadStreamOption` or `opts ...UIMessageStreamOptions`.

## Decisions

### Use typed functional options for utility helpers

The utility helper APIs should use typed functional options, not pointer option structs. The target call shapes are:

```go
modelMsgs, err := aisdk.ConvertToModelMessages(messages)
modelMsgs, err := aisdk.ConvertToModelMessages(messages, aisdk.WithIgnoreIncompleteToolCalls())

err := aisdk.WriteUIMessageStream(w, result)
err := aisdk.WriteUIMessageStream(w, result, aisdk.WithUIMessageStreamSources(true))

stream := result.ToUIMessageStream()
stream := result.ToUIMessageStream(aisdk.WithUIMessageStreamReasoning(false))

for msg := range aisdk.StreamUIMessage(chunks) { /* progressive update */ }
msg, err := aisdk.AssembleUIMessage(chunks, aisdk.WithUIMessageReaderGenerateID(gen))
```

This preserves typed option-family safety while fixing default-call ergonomics. Multiple functional options are meaningful because each option applies one configuration field with normal last-wins behavior for repeated scalar options. This is the same broad style used by `StreamText`, `GenerateText`, provider constructors, registries, and output builders.

The implementation should define separate option interfaces for separate API families so options cannot be accidentally applied to the wrong helper:

```go
type ConvertOption interface { applyConvert(*convertConfig); convertOption() }
type UIMessageStreamOption interface { applyUIMessageStream(*uiMessageStreamConfig); uiMessageStreamOption() }
type UIMessageReaderOption interface { applyUIMessageReader(*uiMessageReaderConfig); uiMessageReaderOption() }
```

Because these are variadic interface parameters, an accidental `nil` option argument is assignable in Go and cannot be rejected at compile time. Builders should therefore ignore nil option values safely while docs, examples, and repository call sites use the intended ergonomic default form with no option arguments. Pointer option structs should still be removed so calls such as `&ConvertOptions{...}` and `&UIMessageStreamOptions{...}` fail to compile.

The previous exported option structs (`ConvertOptions`, `UIMessageStreamOptions`, and `ReadStreamOption`) should be removed from the public API unless an internal unexported config type still needs the same fields. Keeping exported config structs alongside functional options would continue two supported ways to configure the same helper and would weaken the ergonomic cleanup.

Alternative considered: keep pointer option structs or add a `BoolPtr` helper for `UIMessageStreamOptions` booleans, as discussed in #93. Rejected because `nil` defaults, `UIMessageStreamOptions{}` defaults, and pointer-bool fields remain noisy for common helper calls and inconsistent with the rest of the SDK's option-heavy APIs.

Alternative considered: use variadic config structs. Rejected because that was the bug-prone pattern previously removed: callers could pass multiple structs even though the implementation only meant to support one.

### Option names are scoped by API family

Reader-specific options should not reuse the existing orchestration `WithGenerateID` name directly. `WithGenerateID` already configures IDs created by `StreamText`/`GenerateText`, and provider packages also expose their own `WithGenerateID` options. Reader options should use scoped names such as:

- `WithUIMessageReaderGenerateID(fn func() string) UIMessageReaderOption`
- `WithUIMessageStreamGenerateID(fn func() string) UIMessageStreamOption`
- `WithUIMessageStreamSources(send bool) UIMessageStreamOption`
- `WithUIMessageStreamReasoning(send bool) UIMessageStreamOption`
- `WithUIMessageStreamStart(send bool) UIMessageStreamOption`
- `WithUIMessageStreamFinish(send bool) UIMessageStreamOption`
- `WithUIMessageStreamOriginalMessages(messages ...UIMessage) UIMessageStreamOption`
- `WithUIMessageStreamMessageMetadata(fn func(TextStreamPart) json.RawMessage) UIMessageStreamOption`
- `OnUIMessageStreamFinish(fn func(UIMessageStreamOnFinishState)) UIMessageStreamOption`
- `OnUIMessageStreamError(fn func(error) string) UIMessageStreamOption`
- `WithIgnoreIncompleteToolCalls() ConvertOption`

The exact names can be refined during implementation, but they should avoid ambiguous reuse of `WithGenerateID`, `OnFinish`, or `OnError` across unrelated option interfaces.

`WithUIMessageStreamOriginalMessages` must preserve the current `OriginalMessages != nil` sentinel separately from slice length. Calling the option, even with no arguments or with a nil slice expanded via `messages...`, should set an internal `hasOriginalMessages` flag and store a copied zero-length slice. Omitting the option should leave that flag false. This preserves current continuation/message-ID behavior for `UIMessageStreamOptions{OriginalMessages: []UIMessage{}}` and avoids collapsing explicitly empty history into absent history.

### Remove `ReadUIMessageStream` instead of preserving a legacy wrapper

The issue asks to replace the misleading helper with two explicit APIs. Upstream `ai@7.0.11` does not keep a blocking legacy `readUIMessageStream`; its reader is progressive. Keeping a deprecated Go wrapper would preserve a behavioral mismatch and continue exposing a misleading name.

The implementation should remove `ReadUIMessageStream` and migrate repository docs/tests to `StreamUIMessage` or `AssembleUIMessage`. Existing external callers must update:

- use `StreamUIMessage` when they need progressive snapshots; or
- use `AssembleUIMessage` when they need one completed `UIMessage`.

This is intentionally a breaking API cleanup. It follows the project preference to avoid preserving backward compatibility unless explicitly requested and keeps the public API aligned with the two real use cases.

Alternative considered: keep `ReadUIMessageStream` as deprecated one-message/no-error channel wrapper. Rejected because it prolongs the ambiguous naming, is not an upstream-parity behavior, and forces hidden decisions about errors that the old return type cannot expose.

### Factor assembly into incremental state

The implementation should introduce an internal state object for applying one `UIMessageChunk` at a time, rather than building progressive snapshots by replaying all previous chunks on every input. The state should hold the evolving `UIMessage`, active text and reasoning parts keyed by chunk IDs, partial tool-call input state keyed by tool call ID, and data-part indexes by data name/ID. `assembleResponseMessage` can then be replaced or refactored to use the same updater for internal final assembly paths.

The state updater should return package-prefixed errors for malformed stateful sequences such as text delta before text start or tool output before matching tool input. Internal producer-driven final assembly paths, such as `ToUIMessageStream` `OnFinish`, should continue to produce the same final message for valid chunks emitted by this package.

This avoids O(n²) replay behavior and mirrors the upstream `StreamingUIMessageState` pattern while staying idiomatic Go. It also gives `ToUIMessageStream` `OnFinish`, `AssembleUIMessage`, and `StreamUIMessage` one source of truth for message assembly.

Alternative considered: keep `assembleResponseMessage` unchanged and call it after appending each chunk. Rejected because it duplicates ID-generation behavior, performs poorly on long streams, and makes streaming tool input/error handling harder to align with upstream.

### Emit upstream write-point snapshots only

`StreamUIMessage` should emit snapshots only when upstream `ai@7.0.11` would call its write callback for the corresponding chunk and that state is representable in Go. It should not synthesize an additional final snapshot when the input channel closes. Empty streams and streams containing only non-writing chunks close without yielding a message.

Examples of non-writing behavior to preserve include `ChunkError` under upstream's default `terminateOnError=false`, transient data chunks, and upstream non-write lifecycle markers such as step/finish chunks when they do not produce a represented message update. If a finish chunk carries metadata and upstream would write, the Go helper should emit that metadata snapshot. For streams with at least one emitted snapshot, the last received snapshot is the latest complete state produced by upstream write points. `AssembleUIMessage` is intentionally different: it returns the final state after every chunk has been applied, so it equals the last progressive snapshot only when no subsequent non-writing state mutations occurred after that snapshot. For example, a text write point followed by a non-writing `ChunkStartStep` should not cause `StreamUIMessage` to synthesize another snapshot, while `AssembleUIMessage` should include the final `StepStartPart`.

### Handle errors according to API surface

`StreamUIMessage` has no error channel by design because the issue requested `<-chan UIMessage`. For `ChunkError`, it should match upstream's default `terminateOnError=false`: do not emit a snapshot for the error chunk and continue consuming later chunks. If a later valid chunk reaches a write point, it can still emit a snapshot.

Malformed state transitions are different from provider stream error chunks. In strict streaming mode, if the state updater detects an invalid sequence that prevents reliable assembly, `StreamUIMessage` should close its output without emitting a snapshot for the invalid chunk. Callers needing the error value can use `AssembleUIMessage` or consume raw `UIMessageChunk` values directly. `AssembleUIMessage` returns non-nil errors for both `ChunkError` chunks and invalid chunk ordering, because its blocking signature has an explicit error result.

Alternative considered: add `OnError` and `TerminateOnError` reader options now to match more of upstream. Rejected for this issue because it widens reader behavior beyond the requested split. The functional-option shape leaves room to add those options later without another signature change.

### Emit isolated snapshots

Each value sent by `StreamUIMessage` must be a snapshot, not a mutable alias to the internal state. `UIMessage.Parts` contains interface values, `json.RawMessage`, provider metadata maps, nested approval structs, and raw input/output JSON, so a shallow struct copy is not enough. The implementation should add a focused clone helper for `UIMessage` and known `Part` types, including deep copies of raw JSON and metadata maps, and use it before every channel send.

Alternative considered: send the state message directly and document that callers must not mutate it. Rejected because upstream uses `structuredClone`, and aliasing would make progressive snapshots unstable and hard to test.

### Parse partial tool input without invalid RawMessage values

Tool input deltas arrive as accumulated text fragments in `UIMessageChunk.InputTextDelta`, while Go tool parts expose `Input json.RawMessage`. The state must never place an invalid `json.RawMessage` into a snapshot because later marshaling of `UIMessage` could fail.

The implementation should maintain accumulated input text per tool call and port the upstream `parsePartialJson` behavior from `ai@7.0.11`: first try to parse the accumulated text as JSON, then try a Go port of upstream `fixJson` repair logic, and otherwise treat the partial value as unavailable. When parsing or repair succeeds, marshal the parsed value back to valid JSON bytes for `ToolInvocationPart.Input` or `DynamicToolUIPart.Input`. When parsing fails, keep the part in `input-streaming` state with `Input` omitted/nil for that snapshot rather than storing raw invalid text. Each tool-input-delta write point should still be eligible to emit a snapshot; the snapshot simply must not contain invalid JSON.

This is intended as parity-preserving behavior because upstream also parses repaired partial JSON and uses `undefined` when parsing fails. If the Go port cannot match an upstream partial parsing edge case, the implementation should document that case as a parity gap and add a regression test showing that Go still avoids invalid `json.RawMessage`.

### Generate fallback IDs lazily

Progressive snapshots need a message ID before a snapshot is emitted, but custom ID generators can have observable side effects. The reader state should not generate an ID at initialization. Instead, it should:

1. Use a `ChunkStart.MessageID` if it is observed before the first emitted snapshot.
2. Otherwise call a `WithUIMessageReaderGenerateID` generator or `GenerateID()` exactly once immediately before the first emitted snapshot that needs an ID.
3. Reuse that fallback ID for later snapshots unless a subsequent start chunk explicitly supplies a message ID before a later write point, in which case the state follows the stream-provided ID for future snapshots.

`AssembleUIMessage` can delay fallback generation until returning the final message if no stream-provided ID was observed, preserving blocking/final assembly behavior without calling generators unnecessarily.

## Risks / Trade-offs

- [Risk] Migrating utility helpers to functional options broadens the breaking-change surface. → Mitigation: keep the migration mechanical, update all repository call sites/docs, and add compile-oriented tests/examples for the new call shapes.
- [Risk] Removing `ReadUIMessageStream` breaks existing callers. → Mitigation: this is an intentional cleanup; update repository docs/tests and make migration paths explicit: `StreamUIMessage` for progressive snapshots, `AssembleUIMessage` for one final message.
- [Risk] Option names become verbose. → Mitigation: use scoped names to avoid cross-API ambiguity, and prefer default calls with no options for the common case.
- [Risk] `StreamUIMessage` cannot surface errors through its return type. → Mitigation: match upstream default `ChunkError` handling by skipping the error chunk and continuing, return errors from `AssembleUIMessage`, and leave richer streaming error callbacks for a separate proposal.
- [Risk] Progressive state handling can drift from the existing final assembler. → Mitigation: refactor to a shared updater and add tests asserting `AssembleUIMessage` equals the last progressive snapshot only when that snapshot reflects all later state, plus tests where later non-writing state mutations appear only in blocking final assembly.
- [Risk] Snapshot aliasing can let later chunks mutate previously received messages. → Mitigation: deep-copy every snapshot and add tests that mutate an earlier snapshot without affecting later snapshots or internal state.
- [Risk] Upstream exact write points are subtle; not every state mutation emits a snapshot. → Mitigation: compare against `ai@7.0.11` `processUIMessageStream` and add tests for start metadata, text/reasoning lifecycle, tool streaming, data reconciliation, transient data, and finish metadata.
- [Risk] Partial JSON repair can drift from upstream `fixJson`. → Mitigation: port the upstream scanner behavior where feasible and include tests for valid, repaired, and failed partial tool inputs that assert snapshots marshal successfully.
