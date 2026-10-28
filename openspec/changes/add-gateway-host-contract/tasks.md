# Tasks

Each numbered group is one commit. Every commit must leave these passing: `mise run test-ai-gateway`, `mise run test-providerwire-v4` and `go test -race` for `ai-gateway/internal/...`, `ai-gateway/providerwire/v4/...` and `ai-gateway/cmd/grafana-ai-gateway/internal/...`. The branch is based on `nrbrd/gateway-execution-runtime` (#555).

## 1. Host-error contract

- [x] 1.1 Add `ai-gateway/internal/host` with the closed `ErrorCategory` (permission, internal, BYOK discovery) and the `ErrorWriter` interface, and a `doc.go` stub for the adapter contract (design D1). Verify with `go build ./internal/host/...`.
- [x] 1.2 Make ProviderWire implement `host.ErrorWriter`, and remove `HostErrorCategory`, `HostErrorAuthentication` and the host mapping to `safeAuthentication`. Remove `safeAuthentication` if nothing else uses it. Switch `routes.go`, `discovery` and `service` to the interface. Add a table test in `providerwire/v4` asserting byte-identical status, headers and body for each category against the current documents. Verify with `go test ./providerwire/v4/... ./cmd/grafana-ai-gateway/internal/...`.

## 2. Host-owned discovery

- [x] 2.1 Move the discovery document types, sorting, encoding, `schema.json` and their projection tests from `cmd/.../internal/discovery` to `providerwire/v4/discovery.go`, implementing `host.DiscoveryProjection` (design D4). Verify with `go test ./providerwire/v4/...`.
- [x] 2.2 Make `discovery.New(lister, projection, errors)` own the access decision, safe listing and internal failures, and remove `service.ConfiguredDiscovery` and `service.BYOKDiscovery`. Keep the listing-failure, panic, scoped-listing and complete-catalog tests in `discovery`, and add cases for BYOK and refused access with zero listings. Verify with `go test ./cmd/grafana-ai-gateway/internal/discovery/... ./cmd/grafana-ai-gateway/internal/gateway/...` and the configured-discovery TS suite through `mise run test-ai-gateway-command`.

## 3. One selector, one runtime

- [ ] 3.1 Replace `NewConfiguredSelector` and `NewBYOKSelector` with `service.NewSelector`, which branches on account access and takes BYOK dependencies only in `cloud` mode (design D2). Port `selection_test.go` into one table covering configured, BYOK and refused access, with zero catalog and provider calls on refusal, and the 9-account BYOK request mapping to the fixed BYOK 400 through ProviderWire. Verify with `go test -race ./cmd/grafana-ai-gateway/internal/service/...`.
- [ ] 3.2 Build one runtime and one ProviderWire handler in `gateway.go`, and remove `routeHandlers.byokLanguage`, `byAccess` and the second runtime. Update the selection paragraphs in `docs/concepts/gateway-architecture.md`. Verify with `go test ./cmd/grafana-ai-gateway/internal/gateway/...`, `mise run test-ai-gateway-command` and `mise run lint-docs`.

## 4. Route table

- [ ] 4.1 Add the `route` entry type, table validation and `productionRoutes` in `routes.go`. Make `newHandler` register entries behind per-entry access middleware, label from the matched entry, and answer `HEAD` on `GET` entries with 405 (design D3). Add tests for invalid tables (empty label, nil handler or writer, duplicate method and path), and for production routes equal to the manifest operations. Keep `TestGateway_UndeclaredAndLegacyRoutesDoNoWork` and the route-body tests passing, adapted to the table. Verify with `go test ./cmd/grafana-ai-gateway/internal/gateway/... ./cmd/grafana-ai-gateway/internal/manifestdata/...`.
- [ ] 4.2 Describe the route table and host errors in `docs/concepts/gateway-architecture.md`, and add the "host contract" and "route table" rows to `docs/concepts/gateway-glossary.md`. Verify with `mise run lint-docs`.

## 5. Per-attempt option translation

- [ ] 5.1 Export `fallback.DefaultDecider` with a test that it matches the current default for API, context-window, buffer-limit and unknown errors. Verify with `go test ./fallback/...`.
- [ ] 5.2 Add `execution.Target`, `TargetType`, `Translator`, `WithTranslator`, `Translate` and `ErrTranslation`. `Call` stores the translator in the call context, and `ClassifyProvider` maps `ErrTranslation` to internal before its other checks (design D5). Add `execution` tests:
  - no translator passes options through unchanged;
  - the clone isolates attempts;
  - a translator panic and injected call-level `gateway` or protected headers in any case return `ErrTranslation` without calling the inner model, and without the header name or value;
  - a translator's own error is returned unchanged;
  - concurrent calls on one runtime see only their own translator.

  Verify with `go test -race ./internal/execution/...`.
- [ ] 5.3 Wrap every configured candidate in `service/catalog.go`, single-candidate models included, and every BYOK account in `byok/model.go`, as native → `nativeoptions.Model` → `Translate`. Remove BYOK's outer Anthropic wrapper. Give Gateway-built fallback chains a decider that stops on `ErrTranslation`. Add service tests with fake native servers:
  - a heterogeneous Anthropic→OpenAI chain where each request carries only its own translation;
  - BYOK accounts translated per attempt;
  - a panicking translator and an injected protected header on the first candidate, with zero provider requests and no second attempt;
  - translated Anthropic options accepted and refused by native validation on configured and BYOK routes;
  - BYOK Anthropic validation failing every account with zero native I/O and the unchanged document.

  Verify with `go test -race ./cmd/grafana-ai-gateway/internal/service/... ./cmd/grafana-ai-gateway/internal/byok/...`.
- [ ] 5.4 Document `Translator` and the wrapping order in the `execution` godoc, add the "option translator; per-attempt translation" row and reword "Adapter; translator" in the glossary, and add the translation step to the architecture walkthrough. Verify with `mise run lint-docs`.

## 6. Test adapter on a route

- [ ] 6.1 Move `lineAdapter` from `execution_test` to `ai-gateway/internal/testadapter` as an HTTP handler with a line-format outcome and host-error writer and an optional translator (design D6). Keep the `execution_test` parity table passing over it, and add a translation row over a fallback model. Verify with `go test -race ./internal/...`.
- [ ] 6.2 In gateway route tests, register the test adapter on an extra entry next to `productionRoutes`. Assert:
  - refusal in its format without reading the body;
  - its telemetry label;
  - configured and BYOK calls through the shared selector and runtime;
  - a whole body above the former bound reaches the handler;
  - a translator reaches both attempts of a configured fallback model.

  Add a test that `go list -deps ./cmd/grafana-ai-gateway` excludes `internal/testadapter`. Verify with `go test -race ./cmd/grafana-ai-gateway/internal/gateway/...`.
- [ ] 6.3 Complete the adapter contract godoc in `internal/host/doc.go`: what an adapter supplies, what the host provides, and the one-entry registration. Link it from the architecture walkthrough. Verify with `mise run lint-docs`.

## 7. Integration checks

- [ ] 7.1 Run `mise run test-ai-gateway`, `mise run test-ai-gateway-command` (both access modes), `mise run test-providerwire-v4`, `mise run test-race`, `mise run lint`, `mise run lint-docs` and `mise run validate-parity-baseline`. Confirm that no file under `test/conformance/*/recorded/` changed.
- [ ] 7.2 Confirm with `rg` that `NewConfiguredSelector`, `NewBYOKSelector`, `byAccess`, `routeHandlers`, `HostErrorAuthentication`, `ConfiguredDiscovery` and `BYOKDiscovery` appear only in historical references.

## Workflow follow-up

- Open a draft PR titled `refactor(ai-gateway): register adapters through a host contract`, based on `nrbrd/gateway-execution-runtime`, linking #482 and #394.
- Archive the change in the same PR before merge, and verify the synced specs.
