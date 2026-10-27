# Tasks

Each numbered group is one commit that leaves `mise run test-ai-gateway`, `mise run test-providerwire-v4` and `mise run test-grafana` passing. The branch is based on `nrbrd/gateway-resource-policy` (#518).

## 1. No byte bound on streamed provider responses

- [x] 1.1 In `outbound`, pass `text/event-stream` bodies through unwrapped and keep `boundedBody` for every other media type, including a missing or unparsable one (design D4). Update the `NewModelClient` godoc. In `outbound_test.go`, add table cases:
  - a stream totalling more than the bound, plain and gzip-encoded;
  - a single event above the bound, which succeeds;
  - `text/event-stream; charset=utf-8` and mixed-case media types;
  - a non-SSE body and an unparsable media type above the bound, which still fail.

  Verify with `go test -race ./cmd/grafana-ai-gateway/internal/outbound/...`.
- [x] 1.2 In `gateway-command.test.ts`, add a fake Anthropic stream of about 20 MiB in total, in events of about 1 MiB. Assert that both registered clients receive every delta and the finish through the built plugin. Verify with `mise run test-ai-gateway-command`.
- [x] 1.3 Update the transport row and the removed-limits paragraph of the `ai-gateway/README.md` ledger, and the raw-events paragraph of `docs/providers/grafana-gateway.md`, to say that time, not size, bounds a stream, and link #546. Verify with `mise run lint-docs`.

## 2. One provider attempt bound

- [x] 2.1 Add `config.MaxProviderAttempts = 8`. Replace `maxCandidates` with a primary-plus-fallbacks check against it, keeping today's error. Update `file_test.go` so a chain of 8 candidates loads and 9 fails. Verify with `go test ./cmd/grafana-ai-gateway/internal/config/...`.
- [x] 2.2 In `byok`, replace `maxCredentials` with `config.MaxProviderAttempts` in `DecodeRequest` and `New`. Keep the existing tests for 8 and 9 selected accounts, 9 unused accounts, and the handler-level fixed 400 with zero native calls, now against the shared constant. Verify with `go test -race ./cmd/grafana-ai-gateway/internal/byok/... ./cmd/grafana-ai-gateway/internal/service/...`.
- [x] 2.3 Replace the BYOK ledger row with the shared 8-attempt row, and change the catalog row from 15 fallbacks to 7. Verify with `mise run lint-docs`.

## 3. Platform-owned request size

- [x] 3.1 Remove `Settings.MaxRequestBytes`, `defaultRequestBytes`, the `max_request_bytes` parsing and its validation. In `settings_test.go`, assert that `GF_PLUGIN_MAX_REQUEST_BYTES`, set to a valid or an invalid value, is ignored and startup succeeds. Verify with `go test ./cmd/grafana-ai-gateway/internal/config/...`.
- [x] 3.2 Remove the `http.MaxBytesHandler` wrapper and the `maxRequestBytes` parameter from `gateway.newHandler`, and its call in `gateway.go`. Delete the body-bound cases in `request_bound_test.go`, and keep the refused-caller zero-reads test, moving it if the file empties. Add an in-process route test where a request of about 2 MiB with an inline file reaches the fake native provider. Verify with `go test -race ./cmd/grafana-ai-gateway/internal/gateway/...`.
- [x] 3.3 In `gateway-command.test.ts`, test both registered clients through the built plugin:
  - a request of about 2 MiB with an inline file succeeds;
  - a request above 16 MiB fails with a non-ProviderWire host error, and the fake native provider records zero calls.

  Verify with `mise run test-ai-gateway-command`.
- [x] 3.4 Keep `readBody`'s `*http.MaxBytesError` mapping and the testserver's own 1 MiB `MaxBytesHandler`. Update the `providerwire/v4` godoc: a host bounds the body, and the Gateway plugin relies on the route message size. Verify with `go test ./providerwire/v4/...` and `mise run test-providerwire-v4`.
- [x] 3.5 In the ledger, replace the route-wrapper row with the platform request-size row (source: the plugin SDK's 16 MiB receive default) and drop the `max_request_bytes` settings row. In `docs/concepts/gateway-architecture.md`, replace the `max_request_bytes` sentence in the limits paragraph. Verify with `mise run lint-docs`.

## 4. Platform request deadline

- [x] 4.1 Add `RouteDeadline` to `platformhost.Config`, applied in `Host.Handler` before `CallRoute` (design D5). Thread it through the in-process and process hosts. In `host_test.go`, add stub-handler cases for a deadline carried no later than `RouteDeadline`, and for no deadline when it is zero. Verify with `go test -race ./test/platformhost/...`.
- [x] 4.2 Remove `Limits.ModelDuration` and its validation. Make `ServeHTTP` use the request context without a derived timeout. Classify an ended request context by its error (`DeadlineExceeded` timeout, `Canceled` cancellation) in `waitStreamPart` and `streamContextErrorCategory`, matching unary (design D1). Rewrite the `ModelDuration`-based tests in `handler_test.go`, `stream_test.go`, `stream_tools_test.go` and `runtime_test.go` to put the deadline on the request context, and add cases for:
  - deadline expiry giving the timeout category, and cancellation giving the cancellation category, in unary and streaming, before and after commitment;
  - a request without a deadline that runs until finish;
  - selection that consumes part of the deadline, leaving only the remaining time.

  Verify with `go test -race ./providerwire/v4/...`.
- [x] 4.3 Remove `max_model_duration`, `defaultModelDuration` and the "at least 30s" rule from `config`. Assert that `GF_PLUGIN_MAX_MODEL_DURATION`, set to a valid or an invalid value, is ignored. Drop `ModelDuration` from the `Limits` literals in `service`, `gateway` and the testserver. Verify with `go test ./cmd/grafana-ai-gateway/internal/...` and `mise run test-providerwire-v4`.
- [x] 4.4 In the gateway route tests, add cases with a fake native provider that blocks until canceled:
  - with `RouteDeadline`, in unary and streaming mode, the native request is canceled at the deadline, and a two-candidate chain starts no second attempt. Assert only that the call ends, not its timeout or cancellation category;
  - without a deadline, cancelling the caller cancels the native request.

  Verify with `go test -race ./cmd/grafana-ai-gateway/internal/gateway/...`.
- [x] 4.5 Add a `--route-deadline` flag to the test plugin host, and start it with 60s in `gateway-command.test.ts`, as Grafana serves the plugin. Switch the malformed-startup test from `--max-model-duration` to a malformed `agento11y_enabled`, keeping its secret-safety assertions. Verify with `mise run test-ai-gateway-command`.
- [x] 4.6 Update the docs:
  - in the ledger, add the platform request-deadline row (Grafana's route deadline, 60s today, #460), the disabled-deadline paragraph including the unary Anthropic token-budget failure, the note that a platform deadline appears as a cancellation in Gateway telemetry, and the termination grace-period guidance, and drop the `max_model_duration` settings row;
  - in `docs/guides/gateway-authentication.md`, say the 60s deadline includes streams;
  - change "model deadline" to "request deadline" in `docs/concepts/gateway-architecture.md` and `docs/concepts/gateway-glossary.md`;
  - update "Bound work and handle errors" in `docs/providers/grafana-gateway.md`;
  - update the `providerwire/v4` godoc for `Limits`, `Config` and `doc.go`.

  Verify with `mise run lint-docs`.

## 5. Memory owner and ledger completeness

- [x] 5.1 Finish the ledger from design D7:
  - the platform rows for admission, concurrency and process memory, with their sources;
  - the `GOMEMLIMIT` guidance (about 85–90% of the container memory limit);
  - the removed-limits list with replacements.

  Confirm that every retained limit has one owner and every platform limit has a source. Verify with `mise run lint-docs`.
- [x] 5.2 Check `docs/concepts/gateway-architecture.md`, its diagrams and `docs/concepts/gateway-glossary.md` against the resulting code, and fix any remaining drift in limit ownership or deadline wording. Verify with `mise run lint-docs`.

## 6. Integration checks

- [x] 6.1 Run `mise run test-ai-gateway`, `mise run test-ai-gateway-command`, `mise run test-providerwire-v4`, `mise run test-grafana`, `mise run lint-docs`, `mise run validate-parity-baseline`, and `go test -race ./...` in `ai-gateway`. All pass.
- [x] 6.2 Run `rg -w 'ModelDuration|max_model_duration|MaxRequestBytes|max_request_bytes|maxCandidates|maxCredentials' ai-gateway docs --glob '!CHANGELOG.md'`. It finds only the ledger's two removed-setting rows. Confirm that `git diff --stat nrbrd/gateway-resource-policy -- 'test/conformance/*/recorded/'` is empty.
- [x] 6.3 Run `openspec validate delegate-gateway-platform-limits --strict`. It passes.

## Workflow follow-up

- Open the draft PR titled `refactor(ai-gateway)!: delegate request limits to the platform and bound attempts once` against `nrbrd/gateway-resource-policy`, with the review walkthrough from `AGENTS.md`.
- Archive the change in the same PR before merge. After archive, rerun the 6.2 identifier check, including `openspec/specs`.
