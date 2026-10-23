# Tasks

Each numbered group is one commit that leaves `mise run test-ai-gateway`, `mise run test-providerwire-v4` and `mise run test-grafana` passing. The branch is based on `nrbrd/gateway-client-limits` (#479), so the Go client already has no response limits.

## 1. Outbound model client

- [x] 1.1 Rename `outbound.NewAnthropicClient` to `NewModelClient` and update its godoc to "every provider type", the call in `gateway.go`, and `outbound_test.go`, including the renamed invalid-limits test. In `outbound_test.go`, add two tests:
  - a gzip response that is within 16 MiB compressed and above it decompressed fails with `ErrResponseTooLarge`;
  - two sequential responses through one client, each just under the limit, both succeed.

  Verify with `go test -race ./cmd/grafana-ai-gateway/internal/outbound/...`, and confirm `rg NewAnthropicClient ai-gateway docs` finds nothing.
- [x] 1.2 Turn the "Limits" section of `ai-gateway/README.md` into the ledger from design D8. Add the plugin-host and outbound-transport rows plus the trade-off paragraph. Verify with `mise run lint-docs`.

## 2. BYOK selected-provider account bound

- [ ] 2.1 In `byok.DecodeRequest`, keep the non-empty and per-account checks for every supplied array, and apply `maxCredentials` only to the selected provider's array. Keep `byok.New`'s check. Update `TestDecodeRequest_AccountCount` to cover:
  - 8 and 9 accounts on the selected provider;
  - 9 valid accounts on an unused provider;
  - an empty unused array.

  In `service/selection_test.go`, add a handler-level case where 9 selected accounts return the fixed BYOK credentials 400 with zero native requests, and 9 unused accounts succeed. Add the BYOK ledger row. Verify with `go test -race ./cmd/grafana-ai-gateway/internal/byok/... ./cmd/grafana-ai-gateway/internal/service/...`.

## 3. Route-owned request-body bound

- [ ] 3.1 Add `Settings.MaxRequestBytes`, parsed from `max_request_bytes` with the 1 MiB default and required to be positive, and remove `RequestBytes` from `v4.Limits` and `validateLimits`. Update `settings_test.go`: defaults, environment binding, and an invalid value that names the setting. Verify with `go test ./cmd/grafana-ai-gateway/internal/config/...`.
- [ ] 3.2 In `gateway.newHandler`, wrap the route mux once in `http.MaxBytesHandler(mux, MaxRequestBytes)` inside the telemetry middleware. In `routes_test.go`, add three tests:
  - language-model bodies one byte below, at and above the bound, where the over-limit body returns the fixed invalid-request document with zero selector and model calls;
  - a non-ProviderWire body-reading handler, installed through `routeHandlers`, that sees the same bound for configured and BYOK callers;
  - a refused caller whose body reader records zero reads.

  Verify with `go test -race ./cmd/grafana-ai-gateway/internal/gateway/...`.
- [ ] 3.3 In `providerwire/v4`, change `readBody` to `io.ReadAll` plus `Close`. Map `*http.MaxBytesError` to the fixed invalid request and any other read or close error to internal. Move the body-boundary tests in `handler_test.go` behind an `http.MaxBytesHandler` wrapper, and add a failing-reader case that expects internal. State in `doc.go` that hosts own the request-body bound. Verify with `go test -race ./providerwire/v4/...`.
- [ ] 3.4 In `test/providerwire-v4/testserver`, wrap the mux in `main.go` and in the testserver tests with `http.MaxBytesHandler` and a local 1 MiB constant. Drop `RequestBytes` from its three `v4.Limits` literals. Verify with `mise run test-providerwire-v4`.
- [ ] 3.5 Add the route-wrapper ledger row and keep the README settings row for `max_request_bytes` accurate. Verify with `mise run lint-docs`.

## 4. Execution summaries without a source limit

- [ ] 4.1 Remove `sourceBytes` from `execution.Summary` and `execution.Project`, along with `boundedText` and the oversized-body branches. Update `internal/execution` tests:
  - a native message and an error body above 8 MiB keep their message, type, code and status;
  - the malformed and unavailable cases still degrade gracefully.

  Verify with `go test -race ./internal/execution/...`.
- [ ] 4.2 Remove `sourceBytes` from `executionRequest`, `executionView` and `newExecutionRequest` in `execution_capture.go`, together with the `sourceBytes == 0` branch in `current()`. Update `execution.Metadata`'s godoc: it returns nil only for an absent or unencodable overview, and always relocates a native `gateway` value. Rename the unrelated local `sourceBytes` constant in `ai-gateway/test/providerwire-v4/developer-evidence-bounds.test.ts` to `witnessSourceBytes`, leaving its witnesses unchanged. Verify with `go test -race ./providerwire/v4/...` and `mise run test-providerwire-v4`, and confirm `rg sourceBytes ai-gateway` finds nothing.

## 5. Validity helpers

- [ ] 5.1 Without changing behavior, extract the validity parts of the preflights into helpers:
  - metadata: UTF-8 key, UTF-8 value and object prefix;
  - raw usage: UTF-8, JSON and object;
  - warning types and fields;
  - non-empty approval and tool IDs.

  Call the helpers wherever `unarySuccessPreflight`, `streamEventPreflight` and `sourcePreflight` run, so that each preflight is left with size accounting only. Verify with `go test -race ./providerwire/v4/...`, with the existing invalid-UTF-8, non-object-namespace and malformed-raw tests unchanged and passing.

## 6. Unary and HTTP error budgets

- [ ] 6.1 Remove `UnaryResponseBytes`, `unarySuccessPreflight` and `minimumTextPartBytes`, and drop the `limit` parameters of `mapUnarySuccess`, `mapSource` and `validRawUsage`. Until group 7, the stream path keeps its current behavior:
  - `streamEventPreflight` inlines its raw-usage size check, and `maxRawUsageBytes` stays in use only there;
  - source frames stay bounded by the final frame check in `encodeStreamFrame`;
  - the shared helpers `warningsPreflight`, `responseIdentityFits`, `responseStringsFit`, `reasoningFileFits`, `minimumWarningBytes` and `sourcePreflight` stay in use on the stream path only.

  Convert the unary over-limit tests in `response_test.go`, `raw_usage_test.go`, `metadata_test.go`, `sources_test.go`, `generated_file_test.go`, `native_response_test.go` and `function_tools_test.go` into large-success tests:
  - more than 8 MiB of content;
  - many warnings;
  - raw usage above 1,048,576 bytes, including a whitespace-heavy object;
  - many metadata namespaces;
  - a long source ID;
  - a large tool input;
  - large generated media.

  Verify with `go test -race ./providerwire/v4/...`.
- [ ] 6.2 Make `writeUnarySuccess` attach `execution.Metadata` and marshal once. On failure, it writes the fixed internal error with the overview. Remove `maxErrorResponseBytes`. `encodeHTTPError(value, metadata)` encodes once and falls back to the base document only if that fails, and `HostErrorWriter` uses it without a limit. Add tests:
  - a success with an overview above 8 MiB keeps the overview and relocates a native `gateway`;
  - an HTTP failure whose overview is above 64 KiB is written whole;
  - unencodable error metadata falls back to the base document with the same status.

  Update the unary docs in `docs/providers/grafana-gateway.md`: generated files and overview availability. Add the removed unary and error-document rows to the ledger. Verify with `go test -race ./providerwire/v4/...` and `mise run lint-docs`.
- [ ] 6.3 In `test/providerwire-v4/gateway-command.test.ts`, add a configured fake native backend that returns a unary result above 8 MiB and raw usage above 1 MiB, within 16 MiB. Assert that the pinned TS client and the Go client from #479 both receive the complete values through the plugin test host. Verify with `mise run test-ai-gateway-command`.

## 7. Streaming and SSE error budgets

- [ ] 7.1 Remove:
  - `StreamFrameBytes`, `StreamParts`, `streamPartCounter` and `streamWaitPartLimit`;
  - `streamEventPreflight`, `streamWarningCountFits`, `warningsPreflight`, `responseIdentityFits`, `responseStringsFit`, `reasoningFileFits`, `sourcePreflight` and `minimumWarningBytes`;
  - the canonical-frame check in `validateLimits`, and `maxRawUsageBytes`;
  - the tool ID length and tool map size checks in `stream_tools.go`.

  `drainStream` ends only on close or its timer, and `newStreamState` uses a constant initial capacity. Update the testserver and `cmd` `v4.Limits` literals, and the `settings.go` constants. Verify with `go build ./...` in `ai-gateway`.
- [ ] 7.2 Make `encodeStreamFrame` encode once with no size check. The finish attaches `execution.Metadata` before its single encode, and an encoding failure attempts one terminal internal error. `encodeStreamError` builds `error.data` with `providerMetadata` and `nativeError` together, marshals it once, and falls back to the fixed base frame only if that fails. Verify with `go test -race ./providerwire/v4/...`.
- [ ] 7.3 Convert the streaming over-limit and part-count tests in `stream_test.go`, `stream_tools_test.go`, `raw_test.go`, `reasoning_test.go`, `sources_runtime_test.go`, `runtime_test.go` and the `cmd/.../service` tests into acceptance tests:
  - 100,001 tiny parts before finish;
  - a single frame above 1 MiB for text, tool result, source, raw, generated file and finish;
  - a start with many warnings;
  - finish raw usage above 1 MiB;
  - an error frame above 64 KiB;
  - an enriched finish above 1 MiB.

  Add a drain test where a continuously ready channel stops at the drain duration. Keep the invalid-value and lifecycle tests. Verify with `go test -race ./...` in `ai-gateway`.
- [ ] 7.4 Extend the command test from 6.3 with a stream that has a frame above 1 MiB, finish raw usage above 1 MiB and a requested raw event above 1 MiB, and assert both clients receive them through the test host. Update the streaming docs:
  - in `docs/providers/grafana-gateway.md`, the raw-events paragraph, and the "Bound work and handle errors" paragraph, which must no longer say that the Gateway caps stream frames;
  - the handler-bounds paragraph and the SSE wording in `docs/concepts/gateway-architecture.md`;
  - `docs/concepts/gateway-glossary.md`, only where it describes handler byte limits;
  - the `providerwire/v4/doc.go` statement that the handler owns only durations.

  Add the removed frame, part and raw-usage rows to the ledger. Verify with `mise run test-ai-gateway-command` and `mise run lint-docs`.

## 8. Integration checks

- [ ] 8.1 Run `mise run test-ai-gateway`, `mise run test-ai-gateway-command`, `mise run test-providerwire-v4`, `mise run test-grafana`, `mise run lint`, `mise run lint-docs`, `mise run validate-parity-baseline`, Gateway `go test -race ./...` and `openspec validate move-gateway-resource-limits --strict`. All must pass.
- [ ] 8.2 Run `rg -n 'Preflight|UnaryResponseBytes|StreamFrameBytes|StreamParts|maxRawUsageBytes|maxErrorResponseBytes|sourceBytes|NewAnthropicClient' ai-gateway docs --glob '!**/CHANGELOG.md'`. It must find nothing. Then confirm that `git diff --stat nrbrd/gateway-client-limits -- test/conformance` shows no change to recorded or upstream fixture inputs.

## Workflow follow-up

- Open a draft PR titled `refactor(ai-gateway)!: move resource limits to shared boundaries` against `nrbrd/gateway-client-limits`, linking #480 and #394. Its description maps each commit to its spec deltas, and its review includes the before and after diagrams required by `AGENTS.md`.
- Archive the change in the same PR before merge. Then verify that the main specs contain `gateway-resource-policy` and the modified requirements.
