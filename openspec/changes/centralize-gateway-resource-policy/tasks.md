# Tasks

Delivered as one PR on `nrbrd/app-platform` (#428). Each group below is one commit that leaves `mise run test-ai-gateway` and `mise run test-grafana` passing. Groups 5–8 move behavior into the shared runtime and host composition, with ProviderWire's existing handler, race and command tests as the regression contract.

## 1. Route request-body bound

- [ ] 1.1 Move the `max_request_bytes` setting in `cmd/grafana-ai-gateway/internal/config/settings.go` from `Settings.ProviderWire.RequestBytes` to a route-level `Settings.MaxRequestBytes`, keeping its key, 1 MiB default and positive/safe validation. Verify with `settings_test.go` cases for the default and invalid values.
- [ ] 1.2 Wrap the route mux in `internal/gateway/routes.go` once with `http.MaxBytesHandler`, and pass the setting through `gateway.go`. Verify with `routes_test.go` cases through the in-process `test/platformhost`:
  - a non-ProviderWire body-reading handler mounted behind the shared wrapper sees in-limit bodies and an overflow error one byte above;
  - configured and BYOK callers get the same bound;
  - a refused caller with an oversized body gets the permission error without a body read.
- [ ] 1.3 In `providerwire/v4/handler.go`:
  - remove `Limits.RequestBytes`;
  - make `readBody` read to EOF, mapping `*http.MaxBytesError` to the fixed invalid request and other read errors to internal;
  - document host-owned bounds in the package godoc.

  Verify with `handler_test.go` boundary cases (below, at and one byte above, through `http.MaxBytesHandler`) asserting zero selection and model calls on overflow.
- [ ] 1.4 Serve every ProviderWire handler the testserver constructs (`providerwire_v4.go`, `recorded_usage.go`, `structured_output.go` in `test/providerwire-v4/testserver`) behind `http.MaxBytesHandler`, and move the BYOK and file-input byte-boundary tests onto the shared bound. Verify with `mise run test-providerwire-v4`.
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

- [ ] 3.1 Remove the `maxCredentials` check from `byok.DecodeRequest`, keeping empty-array rejection, and keep it in `byok.New`. Map `byok.ErrInvalidRequest` from `New` to `providerv4.ErrInvalidBYOK` in `service.NewBYOKSelector` (the access-aware selector of group 8 keeps this mapping). Verify with:
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

## 5. Shared execution runtime

- [ ] 5.1 Add `execution.Limits` (model, idle and drain durations) with `Validate`. Add `Selector`, `Selection` and `CatalogSelection`, and move the host sentinels (`ErrAccountAccess`, `ErrInvalidBYOK`, `ErrInvalidBYOKSelector`, `ErrUnsupportedGatewayControl`, `ErrReservedProviderOptions`) into `ai-gateway/internal/execution`. Verify with unit tests for limit validation and selection validity: empty ID, nil model, wrong specification version, panic.
- [ ] 5.2 Add `execution.New(Config)`, `Runtime.Begin`, `Call.Generate`, `Call.View` and `Call.Close`, moving `selectModel`, `invokeModel`, `attemptCapture` and `executionRequest.snapshot` out of `providerwire/v4`. Drop `sourceBytes` from `execution.Project`/`Summary` (removing `boundedText` and the oversized-body branches) and make `execution.Metadata` always relocate the native `gateway` value. Verify with `internal/execution` tests:
  - a selector or model that ignores the deadline;
  - panics and `nil, nil` results;
  - late results discarded;
  - attempts after sealing ignored;
  - BYOK attempts captured in order, with no configured overview projected;
  - concurrent requests' captures isolated;
  - a native error body larger than the former 8 MiB budget summarized;
  - unconditional relocation;
  - `go test -race`.
- [ ] 5.3 Add `Call.Stream` and `Stream.Next`/`Accept`/`Close`, moving the setup handoff, late-stream cleanup, `waitStreamPart` and `drainStream` out of `serveStream`. Remove `streamPartCounter`, so drain is bounded by the drain duration only. Verify with `internal/execution` tests for each wait outcome, a permanently ready stream after close, a late stream drained once, invalid setup results, and `go test -race`.
- [ ] 5.4 Add `execution.Classify` with the closed `Category`/`Reason` set, moving `safeErrorFromProvider`/`FromTransport`/`FromResolution`. ProviderWire keeps only the category-to-document table. Verify with a table test over provider statuses, transport timeouts, context errors and host sentinels.
- [ ] 5.5 Switch `providerwire/v4` to the runtime: `Config{Runtime}`, unary through `Begin`/`Generate`, streaming through `Stream` with ProviderWire-owned commitment, state machine and encoding. Remove `Limits.StreamParts` and its config constant. Verify that the existing `providerwire/v4` handler, stream, race and execution tests pass unchanged, except those asserting the removed part counter, which are deleted. Also run `mise run test-ai-gateway-command`.
- [ ] 5.6 Add a test-only minimal adapter (plain-JSON unary and line-framed stream over `execution.Runtime`) under `internal/execution` tests. Verify it gets the same deadline, cancellation, drain, classification and attempt view as ProviderWire for identical fake models.

## 6. Provider output lifecycle validation

- [ ] 6.1 Move `unresolvedProviderCalls` into `Runtime.Begin` as tool-history reconstruction, failing inconsistent history as an invalid request before selection. Verify with `internal/execution` history tests (deferred result, orphan result, conflicting duplicates, approval re-emission) and the existing ProviderWire request tests with unchanged documents.
- [ ] 6.2 Add the stream lifecycle validator in `Stream.Next`, moving the provider lifecycle rules out of `processStreamPart`, `stream_tools.go` and `processReasoningPart`:
  - start position and response-metadata placement;
  - text and reasoning IDs;
  - tool input, call, preview and final-result phases, including history-seeded calls and approvals;
  - finish preconditions;
  - suppression of parts after finish.

  ProviderWire keeps only representability and encoding checks. Verify with `internal/execution` validator tests per rule, the test adapter receiving invalid output at the same part as ProviderWire, and the existing `stream_test.go`/`stream_tools_test.go`/`reasoning_test.go` lifecycle cases passing unchanged.
- [ ] 6.3 Add unary lifecycle validation in `Call.Generate` (call uniqueness, result correlation including history, previews, registered finish reason), moving it out of `mapUnarySuccess`. Verify with `internal/execution` tests and the existing `response_test.go`/`function_tools` unary cases passing unchanged, with no later candidate invoked on invalid output.

## 7. Host request policy and per-attempt translation

- [ ] 7.1 Move `ReservedProviderOptionNamespace`, the protected call-header set and `gateway` control extraction into `Runtime.Begin`. ProviderWire's `mapWireProviderOptions` stops special-casing those names, and `mapWireHeaders` keeps only its duplicate and protocol-header rules. Point `config/file.go` at the execution constant. Verify with:
  - `internal/execution` policy tests: controls removed from model options and passed to the selector; reserved namespaces and protected headers refused with their reasons;
  - the existing ProviderWire tests, which still return the same `reserved provider option namespace` and `protected call header` documents;
  - the test adapter receiving the same policy outcomes.
- [ ] 7.2 Add `execution.Target`, the `Request.Translate` field and `execution.AttemptModel`. Wrap every configured candidate in `service.BuildCatalog` and every BYOK account model in `byok.New` once, directly outside its native option validation and inside fallback. Move BYOK's Anthropic option validation from around the fallback chain onto each account candidate. Verify with tests:
  - a heterogeneous Anthropic→OpenAI fallback chain where each attempt sees only its own translation;
  - BYOK accounts translated per attempt;
  - concurrent requests isolated;
  - a translator that adds a reserved namespace or protected header, or panics, failing that attempt non-retryably with zero provider requests;
  - no translator passing options through unchanged;
  - translated Anthropic options going through native option validation on configured and BYOK routes;
  - single-candidate configured routes translated without a fallback chain.

## 8. Host composition and adapter contract

- [ ] 8.1 Replace `NewConfiguredSelector`/`NewBYOKSelector` with one access-aware `service.NewSelector`, and build one ProviderWire handler in `gateway.go`. Verify with selector tests for configured, BYOK and no access (zero catalog and provider calls on refusal), BYOK attempt-bound mapping, and the existing command suites for both access modes.
- [ ] 8.2 Add `execution.HostErrorWriter` with the permission, internal and BYOK-discovery categories. Make ProviderWire's writer implement it and remove the unused authentication category. Verify with host-error document tests showing unchanged bytes.
- [ ] 8.3 Replace `routeHandlers` and the label switch in `gateway/routes.go` with the route table `{method, path, label, handler, errors}` behind the single `MaxBytesHandler`, with per-entry access middleware and adapter-written refusal. Verify with `routes_test.go`:
  - the table matches the committed manifest paths;
  - a test adapter registered on an extra route gets the body bound, access refusal in its own format, its label and the runtime;
  - undeclared paths and methods do no work.
- [ ] 8.4 Move the ProviderWire discovery document from `cmd/.../internal/discovery` into `providerwire/v4` as a projection, behind a host wrapper that owns listing, panic safety and the access decision. Verify with discovery tests and the configured-discovery TS suites, with unchanged documents and BYOK refusal.
- [ ] 8.5 Replace `Settings.ProviderWire` with `Settings.Execution execution.Limits` plus route-level `MaxRequestBytes`, and validate through `Limits.Validate` without constructing a ProviderWire handler. Verify with `settings_test.go` and `gateway_test.go`.
- [ ] 8.6 Document the adapter contract (runtime, route table, host errors, discovery projection) in the `internal/execution` and `internal/gateway` godoc, and add the execution-runtime owner rows to the README ledger. Verify with `mise run lint-docs` and `go doc` output.

## 9. Unary without adapter budgets

- [ ] 9.1 Replace the following with validity-only helpers called from mapping: `unarySuccessPreflight`, `warningsPreflight`, `sourcePreflight`, `responseIdentityFits`/`responseStringsFit`, custom-part sizing, `minimum*Bytes` and `maxRawUsageBytes`. The helpers check UTF-8, metadata namespace objects, the raw-usage object, custom kinds and timestamps. Remove the `limit` parameters from `mapUnarySuccess`/`mapSource`. Verify with `response_test.go`, `metadata_test.go`, `raw_usage_test.go`, `native_response_test.go`, `reasoning_test.go` and the custom-content tests, converted from over-limit failures to large-success cases with validity failures kept.
- [ ] 9.2 Make `writeUnarySuccess` attach execution metadata and encode once, returning the fixed internal error only when primary encoding fails. Verify with a handler test for a large result plus overview and native `gateway` relocation.
- [ ] 9.3 Remove `UnaryResponseBytes`, its config constant and `maxErrorResponseBytes`. Make `encodeHTTPError` marshal the base plus optional metadata once, falling back to the base document only on marshal failure. Verify with handler and error tests for a failure overview above 64 KiB, and unchanged host error bytes.
- [ ] 9.4 Update the unary TS evidence so both registered clients consume large raw usage, metadata, custom content and failure evidence through the test host. That covers `runtime-integration.test.ts`, `client-consumption.test.ts`, `developer-evidence-access.test.ts` and the testserver kinds. Verify with `mise run test-providerwire-v4` and `mise run test-ai-gateway-command`.

## 10. Streaming without frame budgets

- [ ] 10.1 Remove `StreamFrameBytes`, its config constant and the canonical-frame construction check. Replace `streamEventPreflight`, `streamWarningCountFits`, the `mapStreamWarnings` sizing and `reasoningFileFits` with the shared validity helpers, and encode each event once in `encodeStreamFrame` without a limit. Give the stream-state maps a constant initial capacity. Verify with `stream_test.go`, `stream_tools_test.go`, `reasoning_test.go` and the custom-content stream tests:
  - large events succeed;
  - a start with many warnings succeeds;
  - invalid UTF-8 and invalid metadata still fail.
- [ ] 10.2 Encode finish and SSE error frames once with optional execution metadata and `nativeError`, falling back to the base frame only on encoding failure. Verify with tests for a large finish plus overview, a large event-local summary, and unchanged fixed terminal frames.
- [ ] 10.3 Update `service` stream tests and the streaming TS evidence for large frames, many parts and large stream errors through both clients. Verify with `mise run test-ai-gateway`, `mise run test-providerwire-v4` and `mise run test-ai-gateway-command`.
- [ ] 10.4 Add the removed-limit list with replacements to the README ledger. Verify with `mise run lint-docs`.

## 11. Coverage map and integration checks

- [ ] 11.1 Confirm the single "AI Gateway" row in `test/conformance/PARITY.md` still points at the right evidence (`openspec/specs/gateway-*`, which now includes `gateway-resource-policy` and `gateway-execution-runtime`, and `ai-gateway/` tests), and change it only if its status or evidence location changed. Verify with `mise run validate-parity-baseline`.
- [ ] 11.2 Verify that no adapter-local size or runtime identifiers remain: `rg -n 'Preflight|UnaryResponseBytes|StreamFrameBytes|StreamParts|unaryResponseBytes|streamFrameBytes|streamParts|maxRawUsageBytes|maxErrorResponseBytes|sourceBytes|NewAnthropicClient|anthropic\.response|safeErrorFromProvider|invokeModel|drainStream|routeHandlers|unresolvedProviderCalls|WithChatDefaults' ai-gateway providers/grafana docs openspec/specs` returns only intended historical references.
- [ ] 11.3 Run `mise run check`, `mise run lint`, `mise run test-ai-gateway`, `mise run test-ai-gateway-command`, `mise run test-ai-gateway-source-integration`, `mise run test-providerwire-v4`, `mise run test-integration`, `mise run test-grafana`, `mise run parity-check` and Gateway `go test -race` through `go.gateway.work`. Confirm recorded provider fixtures are unchanged with `git diff --stat -- test/conformance/*/recorded`.
- [ ] 11.4 Run `openspec validate centralize-gateway-resource-policy --strict` and confirm it passes.

## Workflow follow-up

- Open a draft PR against `nrbrd/app-platform` titled `refactor(ai-gateway)!: share the execution runtime and resource policy across adapters`, noting the ProviderWire and `providers/grafana.Limits` API breaks and the adapter contract (runtime, translator, validator, route table) that #248 and #427 rework onto. Retarget it to main after #428 merges.
- Archive this change after review and merge, then rerun strict validation against the merged main specs.
