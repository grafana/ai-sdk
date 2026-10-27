# Tasks

Each numbered group is one commit. Every commit must leave these passing: `mise run test-ai-gateway`, `mise run test-providerwire-v4` and `go test -race` for `ai-gateway/internal/execution/...` and `ai-gateway/providerwire/v4/...`. The branch is based on `main`, after #518 and #532 (#521).

## 1. Selection types, limits and classification in `execution`

- [x] 1.1 Move `Selection`, `RequestSelector`, `CatalogSelector` and the host sentinels (`ErrAccountAccess`, `ErrInvalidBYOK`, `ErrInvalidBYOKSelector`, `ErrUnsupportedGatewayControl`, `ErrReservedProviderOptions`) from `providerwire/v4/selection.go` to `internal/execution` (design D6). Update `cmd/.../service/selection.go`, the testserver and the tests to import them. Verify with `go test ./providerwire/v4/... ./cmd/grafana-ai-gateway/internal/service/...`.
- [x] 1.2 Add `execution.Limits` (idle and drain), its validation and `DefaultLimits` (30s, 1s), and make `v4.Config` take `execution.Limits` (design D7). Add table tests for invalid durations in `execution`. Verify with `go test ./internal/execution/... ./providerwire/v4/...`.
- [x] 1.3 Add the closed `execution.Category` and `execution.Reason` types, with `ClassifySelection`, `ClassifyProvider` and `ClassifyStreamPart` moved unchanged from `safeErrorFrom*` (design D1). Map them into ProviderWire's `safeError`, keeping authentication and BYOK discovery local. Move the classifier tables from `runtime_test.go` to `execution` with the same inputs and expected categories. Keep the ProviderWire document tests unchanged. Verify with `go test -race ./internal/execution/... ./providerwire/v4/...`.
- [x] 1.4 Point the "catalog selection" link in `docs/concepts/gateway-architecture.md` at its new file. Verify with `mise run lint-docs`.

## 2. Attempt capture and execution view in `execution`

- [x] 2.1 Move `attemptCapture`, `executionRequest`, `snapshot` and `executionView` from `execution_capture.go` to `execution` as `Attempts` and `View`, with unchanged behavior, and keep ProviderWire's calls compiling through the moved API. Move their white-box tests from `runtime_test.go` (configured execution, unconfigured native summary, one-candidate synthesis, redaction, seal races) with the same assertions. Verify with `go test -race ./internal/execution/... ./providerwire/v4/...`.

## 3. Runtime with synchronous execution

- [x] 3.1 Add `execution.Runtime`, `Call` and `Stream` (design D1–D3):
  - `New` validates the selector and limits;
  - `Call` selects synchronously with `recover` and validation, and attaches a request-local attempt observer for every selection, BYOK included;
  - `Generate` and `Stream` run on the caller's goroutine, contain panics and `nil, nil`, and let an ended request context win, draining late or errored streams once;
  - `Stream.Next` returns the five wait outcomes and restarts idle each call;
  - `Stream.Close` cancels and starts one bounded drain, and is idempotent.
- [x] 3.2 Add `execution` tests:
  - a selector and a model that return after the deadline, and after cancellation;
  - selector and model panics, invalid selections and `nil, nil`;
  - late unary results discarded, and attempts after sealing ignored;
  - BYOK attempts captured in order and not projected;
  - two concurrent calls with isolated captures;
  - every wait outcome, idle reset by activity, a permanently ready stream after close stopping at the drain duration, a late stream drained once, an errored stream drained once, and a repeated close starting no second drain.

  Verify with `go test -race ./internal/execution/...`.
- [x] 3.3 Make `v4.New` take an `*execution.Runtime` (design D8). Rewrite `ServeHTTP` and `serveStream` over `Call`, `Generate` and `Stream`, keeping commitment, framing, the snapshot after the first wait, and the state machine. Move `validSelection` into the runtime, and delete `selectModel`, `invokeModel`, `callStream`, `waitStreamPart`, `startStreamDrain` and `drainStream`. Verify with `go test -race ./providerwire/v4/...`.
- [x] 3.4 Migrate the ProviderWire tests (design D10):
  - construction and imports change, and expected documents stay byte-identical;
  - tests asserting bounded latency for a selector or model that ignores cancellation become late-result tests with the same expected documents;
  - drain tests move to `execution`.

  Verify with `go test -race ./providerwire/v4/...` and `mise run test-providerwire-v4`.
- [x] 3.5 Construct a runtime per route in `gateway.go` (configured, and BYOK in cloud mode) and in each testserver handler. Verify with `mise run test-ai-gateway` and `mise run test-providerwire-v4`.
- [x] 3.6 Write the `internal/execution` package godoc (what the runtime owns, what adapters own, and that work which ignores cancellation holds its own request), and trim `providerwire/v4/doc.go` to the codec's responsibilities. Update `docs/concepts/gateway-architecture.md` (selection and limits paragraphs, stream-runtime link and the idle and drain owner) and the glossary's ProviderWire, framing and drain rows. Verify with `mise run lint-docs`.

## 4. Limits leave settings

- [x] 4.1 Remove `Settings.ProviderWire`, the stream duration constants from `config` (`limitValidationResolver` already went with task 3.5), so `Validate` constructs no handler. Gateway wiring and `NewModelObservabilityFactories` use `execution.DefaultLimits`. Update `settings_test.go`. Verify with `go test ./cmd/grafana-ai-gateway/internal/...` and `mise run test-ai-gateway-command`.

## 5. Host request policy in the runtime

- [x] 5.1 In `Call`, remove call-level `gateway` from the mapped options and pass its raw JSON to the selector, then refuse protected call headers with the protected-header sentinel before selection (design D4). Move `IsProtectedCallHeader`, its set, and a `ControlNamespace = "gateway"` constant to `execution`. Add `execution` tests for each refusal reason, for `gateway` never reaching the model, and for an ordinary header kept with its case. Verify with `go test -race ./internal/execution/...`.
- [x] 5.2 In ProviderWire:
  - stop deleting `gateway` before mapping, and remove the reserved-namespace loop and the credential check from `mapWireProviderOptions` and `mapWireHeaders`, keeping duplicate spellings and protocol-name dropping;
  - map the protected-header sentinel to the existing document;
  - fix the stale comment that names the removed edge test.

  Keep the protected-header document tests, and keep the reserved-namespace document test for configured call-level `gateway`, including the command test, which now sends call-level `gateway`.
- [x] 5.3 Flip the reserved-namespace tests (design D5):
  - `grafana` and `grafana-ai-sdk` at call level, and `gateway`, `grafana` and `grafana-ai-sdk` on messages, files, tool results, custom parts, reasoning files and MCP tool calls, reach the model unchanged;
  - add a case where a protected header beside an invalid `toolChoice` gets the generic invalid-request document.

  Verify with `go test -race ./providerwire/v4/...` and `mise run test-providerwire-v4`.
- [x] 5.4 In `config/file.go`, refuse an openai-compatible `providerName` whose namespace is `execution.ControlNamespace` instead of `grafana`. Update `file_test.go` so `gateway`, ` gateway` and `gateway.chat` fail and `grafana` loads. Verify with `go test ./cmd/grafana-ai-gateway/internal/config/...`.

## 6. Test-only adapter

- [x] 6.1 In an `execution_test` package, add a minimal adapter over `Runtime` and one scenario table run against both it and `v4.New` with the same fake models (design D9). Each row asserts the category and reason, the model-call and drain counts and the attempt view, and ProviderWire's document for that category. Verify with `go test -race ./internal/execution/...`.

## 7. Integration checks

- [x] 7.1 Run `mise run test-ai-gateway`, `mise run test-ai-gateway-command`, `mise run test-providerwire-v4`, `mise run test-race`, `mise run lint`, `mise run lint-docs` and `mise run validate-parity-baseline`, and confirm that no file under `test/conformance/*/recorded/` changed.
- [x] 7.2 Confirm with `rg` that ProviderWire's `selectModel`, `invokeModel`, `callStream`, `waitStreamPart`, `ReservedProviderOptionNamespace`, `Settings.ProviderWire` and `limitValidationResolver` appear only in historical references. Confirm that the README limit ledger is unchanged.

## Workflow follow-up

- Open a draft PR titled `refactor(ai-gateway)!: extract the shared execution runtime`, based on `main`, linking #481 and #394.
- Archive the change in the same PR before merge, and verify the synced specs.
