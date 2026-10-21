# Tasks

Delivered as one PR on `nrbrd/app-platform` (#428). Each group below is one commit that leaves `mise run test-ai-gateway` and `mise run test-grafana` passing.

## 1. Route request-body bound

- [ ] 1.1 Move the `max_request_bytes` setting in `cmd/grafana-ai-gateway/internal/config/settings.go` from `Settings.ProviderWire.RequestBytes` to a route-level `Settings.MaxRequestBytes`, keeping its key, 1 MiB default and positive/safe validation. Verify with `settings_test.go` cases for the default and invalid values.
- [ ] 1.2 Wrap both route patterns in `internal/gateway/routes.go` with `http.MaxBytesHandler`, inside the access middleware, and pass the setting through `gateway.go`. Verify with `routes_test.go` cases through the in-process `test/platformhost`:
  - a non-ProviderWire body-reading handler mounted behind the shared wrapper sees in-limit bodies and an overflow error one byte above;
  - configured and BYOK callers get the same bound;
  - a refused caller with an oversized body gets the permission error without a body read.
- [ ] 1.3 In `providerwire/v4/handler.go`:
  - remove `Limits.RequestBytes`;
  - make `readBody` read to EOF, mapping `*http.MaxBytesError` to the fixed invalid request and other read errors to internal;
  - document host-owned bounds in the package godoc.

  Verify with `handler_test.go` boundary cases (below, at and one byte above, through `http.MaxBytesHandler`) asserting zero selection and model calls on overflow.
- [ ] 1.4 Wrap the ProviderWire testserver handler (`test/providerwire-v4/testserver/providerwire_v4.go`) with `http.MaxBytesHandler`, and move the BYOK and file-input byte-boundary tests onto the shared bound. Verify with `mise run test-providerwire-v4`.
- [ ] 1.5 In `ai-gateway/README.md`:
  - start the limit ledger in "Limits" with the plugin-host and route-handler rows, describing `max_request_bytes` as applying to every route;
  - add the trade-offs from gateway-resource-policy.

  Verify with `mise run lint-docs`.

## 2. Shared outbound transport

- [ ] 2.1 Rename `outbound.NewAnthropicClient` to `NewModelClient`, keeping the fixed `config.ProviderResponseBytes`/`ProviderResponseHeaderTimeout` constants, and update `gateway.go` and its tests. Verify with `mise run test-ai-gateway`.
- [ ] 2.2 Add outbound transport tests:
  - a gzip-compressed response below the bound that decompresses above it fails with `ErrResponseTooLarge`;
  - each request gets an independent bound.

  Verify with `go test ./cmd/grafana-ai-gateway/internal/outbound/...`.
- [ ] 2.3 Add the outbound ledger rows to `ai-gateway/README.md`. Verify with `mise run lint-docs`.

## 3. BYOK selected-provider attempt bound

- [ ] 3.1 Remove the `maxCredentials` check from `byok.DecodeRequest`, keeping empty-array rejection, and keep it in `byok.New`. Map `byok.ErrInvalidRequest` from `New` to `providerv4.ErrInvalidBYOK` in `service.NewBYOKSelector`. Verify with:
  - `byok/request_test.go`: an unused provider with 9 accounts is accepted;
  - `byok/model_test.go`;
  - a selector test: 9 selected accounts return the fixed BYOK credentials 400 with zero native calls.
- [ ] 3.2 Update the BYOK account-count coverage in the TS command suites and add the BYOK ledger row. Verify with `mise run test-ai-gateway-command` and `mise run lint-docs`.

## 4. Go client limits

- [ ] 4.1 Replace the `providers/grafana.Limits` fields with `DiscoveryBytes` and `ResponseBytes` (defaults 4 MiB and 64 MiB) and update constructor validation. `ResponseBytes` covers unary bodies, model-route errors and the cumulative SSE stream; `DiscoveryBytes` covers discovery success and errors. Verify with updated default and invalid-limit tests in `provider_test.go`.
- [ ] 4.2 Remove the per-event and event-count checks from `eventReader` and the `maxRawUsageBytes` cap from `wireUsage`, keeping pre-decode UTF-8/JSON validation. Verify with `stream_test.go`/`model_test.go` cases and `stream_leak_test.go`:
  - an event above 1 MiB;
  - more than 100,000 events;
  - raw usage above 1 MiB;
  - cumulative overflow one byte above the limit;
  - invalid UTF-8 still rejected.
- [ ] 4.3 Update the limit guidance in `docs/providers/grafana-gateway.md` and the godoc for `Limits`/`DefaultLimits`, including the independent-client rationale. Verify with `cd providers/grafana && go test -race ./...` and `mise run lint-docs`.

## 5. Execution projection and unary without adapter budgets

- [ ] 5.1 Drop the `sourceBytes` parameter from `execution.Project` and `execution.Summary`, removing `boundedText` and the oversized-body branches. Make `execution.Metadata` always relocate the native `gateway` value. Remove `sourceBytes` from `executionRequest`/`executionView`. Verify with updated `internal/execution` and `providerwire/v4` execution tests, including a native error body larger than the former 8 MiB budget and unconditional relocation.
- [ ] 5.2 Replace the following with validity-only helpers called from mapping: `unarySuccessPreflight`, `warningsPreflight`, `sourcePreflight`, `responseIdentityFits`/`responseStringsFit`, custom-part sizing, `minimum*Bytes` and `maxRawUsageBytes`. The helpers check UTF-8, metadata namespace objects, the raw-usage object, custom kinds and timestamps. Remove the `limit` parameters from `mapUnarySuccess`/`mapSource`. Verify with `response_test.go`, `metadata_test.go`, `raw_usage_test.go`, `native_response_test.go`, `reasoning_test.go` and the custom-content tests, converted from over-limit failures to large-success cases with validity failures kept.
- [ ] 5.3 Make `writeUnarySuccess` attach execution metadata and encode once, returning the fixed internal error only when primary encoding fails. Verify with a handler test for a large result plus overview and native `gateway` relocation.
- [ ] 5.4 Remove `Limits.UnaryResponseBytes`, its `unaryResponseBytes` constant in config, and `maxErrorResponseBytes`. Make `encodeHTTPError` marshal the base plus optional metadata once, falling back to the base document only on marshal failure. Update `HostErrorWriter` and config validation. Verify with:
  - handler and error tests for a failure overview above 64 KiB;
  - unchanged host error bytes;
  - `settings_test.go`.
- [ ] 5.5 Update the unary TS evidence so both registered clients consume large raw usage, metadata, custom content and failure evidence through the test host. That covers `runtime-integration.test.ts`, `client-consumption.test.ts`, `developer-evidence-access.test.ts` and the testserver kinds. Verify with `mise run test-providerwire-v4` and `mise run test-ai-gateway-command`.

## 6. Streaming without frame and part budgets

- [ ] 6.1 Remove `Limits.StreamFrameBytes`, `Limits.StreamParts`, their config constants, the canonical-frame construction check and `streamPartCounter`. Bound `drainStream` by `StreamDrainDuration` only, and give the stream-state maps a constant initial capacity. Verify with:
  - `stream_test.go` drain tests: a permanently ready channel stops within the drain duration, and a late setup stream is drained once;
  - `go test -race ./providerwire/v4/...`.
- [ ] 6.2 Replace `streamEventPreflight`, `streamWarningCountFits`, the `mapStreamWarnings` sizing and `reasoningFileFits` with the shared validity helpers, and encode each event once in `encodeStreamFrame` without a limit. Verify with `stream_test.go`, `stream_tools_test.go`, `reasoning_test.go` and the custom-content stream tests, converted to these cases:
  - large events succeed;
  - a start with many warnings succeeds;
  - invalid UTF-8 and invalid metadata still fail.
- [ ] 6.3 Encode finish and SSE error frames once with optional execution metadata and `nativeError`, falling back to the base frame only on encoding failure. Verify with tests for a large finish plus overview, a large event-local summary, and unchanged fixed terminal frames.
- [ ] 6.4 Update `service` stream tests and the streaming TS evidence for large frames, many parts and large stream errors through both clients. Verify with `mise run test-ai-gateway`, `mise run test-providerwire-v4` and `mise run test-ai-gateway-command`.
- [ ] 6.5 Add the execution-runtime rows and the removed-limit list with replacements to the README ledger. Verify with `mise run lint-docs`.

## 7. Coverage map and integration checks

- [ ] 7.1 Update the Gateway rows in `test/conformance/PARITY.md` (request-only engine, provider metadata, execution overviews, runtime and Go client) to describe the shared resource policy instead of the transitional envelope limits. Verify with `mise run validate-parity-baseline`.
- [ ] 7.2 Verify that no adapter-local size identifiers remain: `rg -n 'Preflight|UnaryResponseBytes|StreamFrameBytes|StreamParts|unaryResponseBytes|streamFrameBytes|streamParts|maxRawUsageBytes|maxErrorResponseBytes|sourceBytes|NewAnthropicClient|anthropic\.response' ai-gateway providers/grafana docs openspec/specs` returns only intended historical references.
- [ ] 7.3 Run `mise run check`, `mise run lint`, `mise run test-ai-gateway`, `mise run test-ai-gateway-command`, `mise run test-ai-gateway-source-integration`, `mise run test-providerwire-v4`, `mise run test-integration`, `mise run test-grafana`, `mise run parity-check` and Gateway `go test -race` through `go.gateway.work`. Confirm recorded provider fixtures are unchanged with `git diff --stat -- test/conformance/*/recorded`.
- [ ] 7.4 Run `openspec validate centralize-gateway-resource-policy --strict` and confirm it passes.

## Workflow follow-up

- Open a draft PR against `nrbrd/app-platform` titled `refactor(ai-gateway)!: centralize transport resource policy across adapters`, noting the ProviderWire and `providers/grafana.Limits` API breaks. Retarget it to main after #428 merges.
- Archive this change after review and merge, then rerun strict validation against the merged main specs.
