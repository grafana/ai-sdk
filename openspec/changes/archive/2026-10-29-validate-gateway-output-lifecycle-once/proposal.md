# Proposal

## Why

#609 made the execution runtime authoritative for provider output lifecycle rules: `Stream.Next` returns `WaitInvalidOutput`, and `Call.Generate` returns `ErrInvalidOutput`, so an invalid part never reaches ProviderWire. ProviderWire V4 still runs the same rules a second time through its own `StreamValidator` and `ValidateGenerateResult` (archived change `2026-10-29-share-gateway-output-lifecycle-validation`, D8, option A). That second run can never reject anything in production. It exists only so that ProviderWire's white-box tests, which call its internals without the runtime, kept passing unchanged. Every rule runs twice, ProviderWire keeps lifecycle state it does not need, and `lifecycle_agreement_test.go` exists only to prove that the two runs agree. This change removes the second run (#621, part of #394, after #609). It is option B from that design's Open Questions.

## What Changes

- **Stream.** `streamState` no longer holds a `StreamValidator` or a `phase`, and `processStreamPart` no longer calls a validator. A first `stream-start` delivered by the runtime is always valid, so `runStream` branches on the part type alone. The clean close already comes from `WaitFinished` versus `WaitClosed`; the `segmentPhase` projection and its constants are removed.
- **Unary.** `mapUnarySuccess` no longer calls `ValidateGenerateResult`, and `unaryMappingContext` drops its `history` field. `Call.Generate` already rejects invalid output before ProviderWire maps it.
- **History plumbing.** The handler no longer reads the call's tool history, and `serveStream` and `runStream` drop their history parameter. `Call.ToolHistory()` loses its only consumer and is removed (see design D3). The runtime keeps the history internally and seeds its own validators as before.
- **ProviderWire keeps** its representation checks, its content-validity checks (source, custom kind, file data, token counts, response identity, recovered-panic refusal) and its plumbing, all unchanged. Its `default` case still refuses a part type it cannot encode.
- **Tests.**
  - The white-box tests that assert a lifecycle rejection or the segment phase through `newStreamState`, `processStreamPart` or `mapUnarySuccess` lose those assertions. Each assertion is kept by an `execution` rule-table row or by a ProviderWire handler test through the runtime. Calls that only exercise representation stay as they are.
  - `lifecycle_agreement_test.go` is replaced by a handler-level test: every stream and unary case it held runs through the runtime harness. A runtime rejection at part *i* must write the same bytes as a ProviderWire-owned content rejection at part *i*.
  - The test-only `unresolvedProviderCalls` wrapper is removed. `TestRuntimeProviderToolHistory` reads the pending calls with `execution.RebuildToolHistory`, and its ambiguous-history companion moves to the `execution` history table.
- **Docs.** The execution package godoc stops describing an adapter that evaluates lifecycle rules with its own `StreamValidator`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-execution-runtime`:
  - **Adapter representation checks:** adapters no longer evaluate output lifecycle rules internally; they rely on the runtime's outcome. The "Agreement at the same part" scenario now says that ProviderWire writes, at the part where the runtime rejects, the same response as for its own content rejection.
  - **Prompt tool history reconstruction:** the call no longer exposes a history accessor. Each validator still uses its own copy.

## Impact

- **Code:** `ai-gateway/providerwire/v4` (`stream.go`, `response.go`, `handler.go` and the white-box tests) and `ai-gateway/internal/execution` (removing `Call.ToolHistory`, the godoc, and rule-table rows wherever a moved assertion is not already covered). `internal/testadapter` does not change.
- **Go API:** internal only. `execution.Call.ToolHistory` is removed. `StreamValidator`, `NewStreamValidator`, `ValidateGenerateResult` and `RebuildToolHistory` stay exported for the runtime and its tests; ProviderWire tests may still call `RebuildToolHistory` to inspect a mapped prompt.
- **Wire:** no change. Every ProviderWire status, document, SSE frame and execution overview stays byte-identical, including the part at which an invalid stream ends and the frames written before it.
- **Parity:** this is a Gateway-internal refactor with no counterpart in the `@ai-sdk/*` packages pinned in `test/conformance/upstream.yaml`. The LanguageModelV4 stream-part contract, `upstream.yaml`, `knownGaps`, the fixtures, the expectations and the PARITY.md "AI Gateway" row do not change.
- **Delivery:** base on `main` after #609 merges. Until then, the branch builds on `nrbrd/gateway-lifecycle-validation`. Draft PR title: `refactor(ai-gateway): validate provider output lifecycle once`. This change is archived in the same PR.
- **Out of scope:** changing any lifecycle rule; moving the content-validity checks into the runtime; unexporting the execution validators; any change to ProviderWire's wire output.
