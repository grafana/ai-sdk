# Tasks

Each numbered group is one commit. In every group, write the tests first and confirm they fail. After each group, `cd ai-gateway && go test ./...` and `mise run test-grafana` must pass. Do not record, hand-write or edit provider `input*.chunks.txt` fixtures.

## 1. Delivery order (client first)

- [ ] 1.1 Deliver as two PRs, following design "Confirmed decisions":
  - **PR A, client:** group 5 and task 6.3 (raw SSE wire against the Go client only). Release `providers/grafana` through its normal release path.
  - **PR B, Gateway:** groups 2, 3 and 4, tasks 6.1 and 6.2, and group 7. Start after PR A merges; ship after the client release. Its contract tests build the client from source (`ai-gateway/go.mod` replace), and it bumps the `providers/grafana` requirement in `ai-gateway/go.mod` to the released version.

  Verify: each PR passes its own checks on `main`. PR B's description names the minimum Go client version.

## 2. Status-less provider stream error on the server

- [x] 2.1 Add the failing tests:
  - In `internal/execution/classify_test.go`: change the `"zero status"` row (`:103`) so its stream column is `CategoryProviderStream`. Add stream rows for a status-0 error with a plain cause (still provider stream), a wrapped deadline (timeout) and a wrapped cancellation (cancellation). Keep every unary column unchanged.
  - In `providerwire/v4/schema_test.go`: add `canonicalProviderStreamErrorFrame` next to the other frames (`:56`). Add `TestStreamEventSchema` cases:
    - valid: the status-less document;
    - invalid: `provider_error` with any `statusCode`, with `retryable:true`, or with a different message or type;
    - invalid: every existing code without `statusCode`, not only `upstream_error`.
  - In `providerwire/v4/stream_test.go` ("api error status and wrapped transport are classified safely"): add a status-0, plain-cause row that expects the exact base fields with no `statusCode`, the governed `nativeError`, the native message absent from base fields, and later output and finish preserved.
  - In `providerwire/v4/runtime_test.go` `TestPublicError_FixedBytes` (`:856-889`): add `{"provider stream", safeError{category: safeProviderStream}, 500, canonicalInternalError, canonicalProviderStreamErrorFrame}`. The row proves HTTP → internal 500 and stream → `provider_error` frame.

  Verify: `cd ai-gateway && go test ./internal/execution/... ./providerwire/v4/...` fails only on these cases.
- [x] 2.2 Implement design D1–D3:
  - append `execution.CategoryProviderStream` with its godoc after `CategoryInternal`;
  - make it the status-0 fallback in `ClassifyStreamPart`;
  - append `safeProviderStream` after `safeBYOKDiscovery` and add its `safeCategories` entry;
  - add `codeProviderError` and the `publicErrors` definition (status 0, `internal_server_error`, `retryable:false`);
  - add the category to the `streamErrorResponse` allowlist;
  - in `encodeHTTPError` (`error_response.go:198-207`) only, map the provider-stream category to `safeInternal` before resolving the definition. Leave `definitionForError` a plain lookup, because `streamErrorResponse` calls it (`:215`);
  - in `schema/stream_event.json`:
    - add `provider stream error` to the message enum and `provider_error` to the code enum;
    - remove `statusCode` from `error.required`, and add `"required":["statusCode"]` to each status-bearing per-code `then` (or one conditional rule for every code other than `provider_error`);
    - add a `provider_error` `allOf` entry whose `then` has `"not":{"required":["statusCode"]}` plus const message, type and `retryable:false`.

  Verify: `cd ai-gateway && go test -race ./internal/execution/... ./providerwire/v4/...` passes.
- [x] 2.3 In `internal/execution/adapter_test.go`, add an "in-band status-less stream error" scenario that expects `PartErrors: [CategoryProviderStream]` and ends with a valid finish, because `assertProviderWire` requires a finish when `Category == 0` (`:136`). Add its `unaryDocuments` entry `{0, "provider_error", "provider stream error"}`. Verify: `cd ai-gateway && go test ./internal/execution/...` passes.
- [x] 2.4 Update `docs/providers/grafana-gateway.md` ("Bound work and handle errors", retry guidance at `:378-381`). Say that a stream error can carry code `provider_error` with no status, that it is not retryable, so `*provider.APICallError.IsRetryable` is false and root `fallback` with `WithBufferedStream` does not fall back on it, and that later parts remain readable. Verify: `mise run lint-docs` passes.

## 3. Clean EOF after a provider error

- [x] 3.1 Add failing `providerwire/v4/stream_test.go` cases, next to "premature eof is terminal without synthetic finish or block end" (`:454`), for the spec scenarios of "Clean end after a provider error":
  - an open tool input, then an error, then close: clean EOF, with no `internal_error`, `tool-input-end` or finish;
  - an error as the first part, then close;
  - an error, then `text-start`, then close: exactly one `internal_error`;
  - an error, then an unrequested raw part, then close: clean EOF;
  - an error, then an idle timeout: the timeout document;
  - a close with no error: unchanged.

  Update the service rows in `cmd/grafana-ai-gateway/internal/service` whose stream ends with a provider error frame and then closes:
  - `fallback_acceptance_test.go:52` ("tool input then error"): expect `[stream-start, tool-input-start, error]` and `["overloaded"]`;
  - `fallback_acceptance_test.go:286` ("postcommit_error_close"): expect `[stream-start, error]`;
  - `generated_file_test.go:29` ("file then error"): expect `[stream-start, file, error]`.

  Verify: the new and updated cases fail.
- [x] 3.2 Implement design D5 with a `streamState` flag that is set after a written provider error frame and cleared after any other written frame, and checked on `WaitClosed`. Verify: `cd ai-gateway && go test -race ./providerwire/v4/... ./cmd/grafana-ai-gateway/internal/service/...` passes, including `stream_tools_test.go` "premature EOF".
- [x] 3.3 Update `docs/concepts/gateway-architecture.md` ("Streaming, failures, and the point of no return"):
  - in the premature EOF sentence, say that a stream that ends right after a provider error event ends cleanly;
  - in the flowchart (`:199-213`), add an edge from `read` to `cleanup` labelled "provider closed right after a provider error event".

  Check the glossary rows "Error event; terminal failure" and "Execution runtime" in `docs/concepts/gateway-glossary.md` and update them only if they no longer match. Verify: `mise run lint-docs` passes.

## 4. Value-less raw events

- [x] 4.1 Add failing tests:
  - in `providerwire/v4/raw_test.go`: the requested-raw expectations write `{"type":"raw"}` at the position of the value-less part (`:115-116`). A whitespace-only value is also value-less. Invalid UTF-8 and duplicate-key values are still dropped. Unrequested parts are still absent.
  - in `schema_test.go`: move `{"type":"raw"}` from the `TestStreamEventSchema` invalid list (`:160`) to the valid list.

  Verify: the new expectations fail.
- [x] 4.2 Implement design D4: add `omitempty` to `streamRawEvent.RawValue`, accept the empty value in `validStreamEvent` and the `PartRaw` branch, and set `raw.required` to `["type"]` in `schema/stream_event.json`. Update the `projectRawValue` godoc (`raw.go:23-29`), which still gives the old drop rationale for value-less parts. Verify: `cd ai-gateway && go test -race ./providerwire/v4/...` passes.
- [x] 4.3 Update `docs/providers/grafana-gateway.md` ("Inspect raw provider events"). A native event that is not valid JSON now arrives as a raw part without a value. One that repeats a key or is not valid UTF-8 is skipped. Verify: `mise run lint-docs` passes.

## 5. Go client decoding

- [x] 5.1 Add failing tests in `providers/grafana`:
  - in `errors_test.go`: the status-less document becomes a `PartError` with `StatusCode` 0, `IsRetryable` false, the public message, and `Data` kept exactly. `"statusCode": null` is accepted for `provider_error` only.
  - These all yield one terminal protocol `PartError`:
    - `provider_error` with an integer `statusCode`, including an explicit `0`;
    - `provider_error` with `retryable` missing or set to true;
    - `upstream_error` without `statusCode` or with `"statusCode": null`;
    - an HTTP 500 envelope with code `provider_error`.
  - in `stream_test.go`: remove `{"type":"raw"}` from the invalid list (`:367`) and add a case where it yields a `PartRaw` with a nil value when `IncludeRawChunks` is true and is filtered otherwise.
  - in `TestModel_StreamEOF`: EOF directly after a status-less error closes cleanly.

  Verify: `mise run test-grafana` fails only on these cases.
- [x] 5.2 Implement design D6: make `wireStreamError.StatusCode` an `*int`, accept the status-less registry entry only for SSE, set `IsRetryable` explicitly to false, and accept an absent `rawValue`. Add no exported symbol. Verify: `mise run test-grafana` and `cd providers/grafana && go test -race ./...` pass (`mise run test-race` does not cover this module). `go doc github.com/grafana/ai-sdk/providers/grafana` shows no API difference from `main`.

## 6. Pinned-client contract evidence

The Go capture serializes `APICallError.statusCode` as `0` (no `omitempty` at `provider/api_call_error.go:30`; whole parts emitted at `providers/grafana/internal/capture/main.go:184`), while the TS client has no `statusCode`. Every cross-client normalizer in 6.1 and 6.3, including the one at `go-client-differential.test.ts:467`, must treat `0` as absent.

- [ ] 6.1 Add a status-less testserver scenario in `ai-gateway/test/providerwire-v4/testserver/providerwire_v4.go` and register it in the model list (`:473`). It emits, in order:
  - stream-start;
  - a value-less raw part;
  - a status-0 error with a hostile native message;
  - a text block;
  - a value-less raw part;
  - a status-0 error;
  - close without finish.

  Add a `runtime-integration.test.ts` test for raw requested and not requested. Both clients must get the same part order, the fixed base fields with no `statusCode` and `retryable` false, no native text in base fields, a governed `nativeError` only if present, no trailing internal error, and frames valid under `validateStreamEvent`. Verify: `mise run test-ai-gateway-source-integration` passes.
- [x] 6.2 Update `runtime-integration.test.ts:720-746`. The existing `raw` scenario's value-less part (`testserver/providerwire_v4.go:282`) now produces three requested raw events: both clients see a value-less raw in second position and the HTTP frame count is 3. Verify: `mise run test-providerwire-v4` passes.
- [x] 6.3 Extend `go-client-differential.test.ts`, next to the fixture at `:457-472`, with a raw SSE wire containing the status-less error, a value-less raw event and EOF right after a final error. Both clients must normalize to the same parts, and the Go client must report no error. Verify: `mise run test-providerwire-v4` passes.

## 7. Integration checks

- [ ] 7.1 Run `mise run fmt`, `mise run vet`, `mise run lint`, `mise run test`, `mise run test-race`, `mise run parity-check` (`mise.toml:421-428`: `validate-parity-baseline`, `parity-coverage`, `parity-provider-shape`, `test-providerwire-v4`, `test-conformance`) and `mise run test-ai-gateway-source-integration`. All must pass. Provider goldens and the parity baseline stay unchanged (`git diff --stat test/conformance` is empty).
- [ ] 7.2 Run `openspec validate gateway-status-less-stream-errors-and-valueless-raw --strict`. It must pass.

## Workflow follow-up

- Choose each PR title release type (`providers/grafana` for PR A, `ai-gateway` for PR B) per CONTRIBUTING "Releases and independent modules". Follow the delivery order in task 1.1.
- Archive this change in PR B, the last implementing PR, as CONTRIBUTING requires for PRs that touch `openspec/`.
