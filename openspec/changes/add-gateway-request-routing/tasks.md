# Tasks

## 1. Model-group attribution (no behavior change)

- [x] 1.1 Replace `configuredSelection` and `RequestAccounts` in `ai-gateway/internal/execution` with ordered model groups (D4). Update `CatalogSelector`, `AccountSelection`, `Project`/`projectAccounts` and the BYOK selector. Verify that the existing execution, capture, BYOK and test-adapter tests pass unchanged and that the overview JSON is byte-identical.
- [x] 1.2 Add projection unit and race tests for multi-group configured and BYOK decisions, including position mismatches that omit the overview. Verify with `go test -race ./internal/execution/...` in `ai-gateway`.

## 2. Routing classification and documents

- [x] 2.1 Add the invalid-routing category, its reasons and the fallback-model reason to `execution` classification (D8), with classifier table tests. Verify with `go test ./internal/execution/...`.
- [x] 2.2 Add the fixed ProviderWire routing documents and the new unsupported-control message in `providerwire/v4`, with byte tests for each document and unchanged bytes for all other documents. Verify with `go test ./providerwire/...`.

## 3. Control decoding

- [x] 3.1 Implement the exact-key control decoder with per-access allowed sets and the spec's diagnostic precedence (D1). Cover malformed, conditional, empty, case-variant and unsupported (`sort`, `has`, `zeroDataRetention`, ...) cases. Verify with focused service tests.
- [x] 3.2 Extend `byok.DecodeRequest` to return ordered models for the selector and its `models` entries, keeping full-map validation and the existing BYOK credential and selector errors. Verify with byok decoding tests, including fallback providers without accounts and invalid fallback selectors.

## 4. Configured routing

- [x] 4.1 Make `buildCatalog` return the service-local physical route table (D2), sharing instances with the prebuilt chains. Verify with a catalog test asserting that each candidate is constructed once and is the same instance as in the route chain.
- [x] 4.2 Implement the pure composition function (D3): `only`, then stable `order`, skipped repeats, no-candidates and attempt-bound errors. Verify with table tests covering every `gateway-request-routing` composition scenario.
- [x] 4.3 Wire configured routing into `service.NewSelector`: resolve names through the catalog, compose, add the decider and physical observer, and use the configured factory under the primary's canonical ID. Keep the prebuilt model when there are no controls. Verify with service tests using fake Anthropic, OpenAI and compatible HTTP: attempt order, native request bodies per candidate, one logical observation, physical records with composed indexes, cancellation and committed stream errors.

## 5. BYOK routing

- [x] 5.1 Compose BYOK multi-model chains in `byok.New` (D6), with the request-level Anthropic check only for all-Anthropic chains. Verify with fake native tests: cross-provider fallback with only its own key, no configured account used, and a mixed chain where the per-candidate refusal still has no provider I/O.
- [x] 5.2 Implement the BYOK provider start timeout wrapper (D7) and decode `providerTimeouts.byok`. Verify with fake servers that delay response headers in unary and streaming modes: fall-through, no effect after the first byte, last-attempt timeout category, invalid maps, and request cancellation winning a race (`-race`).

## 6. Routing metadata

- [x] 6.1 Project `gateway.routing` in the attempt view and place it through `execution.Metadata` on every carrier (D5). Verify with unit tests for each routing-summary scenario in the `gateway-execution-overview` delta, and ProviderWire unary, error, finish and committed-error tests.
- [x] 6.2 Add the optional `routing` definition to `providerwire/v4/schema/gateway_execution.json` and to the strict TypeScript schema test. Verify with `mise run test-providerwire-v4`.
- [x] 6.3 Update the existing exact-metadata expectations that now include `gateway.routing`, and check that only that member was added. Verify with `mise run test` in `ai-gateway`.

## 7. Go client routing support

- [x] 7.1 Add `GatewayOptions`, `BYOKAccount` and `ProviderTimeouts` to `providers/grafana` (D9), with godoc. Verify with unit tests for nil versus empty `Only`, omitted account fields and body equivalence with raw `gateway` options, plus the updated public-surface and module-isolation tests (`cd providers/grafana && go test ./...`).
- [x] 7.2 Add `GatewayMetadataFrom` and `GatewayMetadataFromError` and their typed overview and routing values (D10), with godoc. Verify with unit tests for absence, malformed members, large numeric codes and string codes, and for unary result, stream finish, unary error document and stream error part carriers.
- [x] 7.3 Add a logger-capture test in `test/integration/testserver` (the module that already depends on both the Grafana client and `middleware/logger`) showing that typed BYOK `apiKey` values are redacted while other account fields and `providerTimeouts.byok` are kept. Verify with `cd test/integration/testserver && go test ./...`.
- [x] 7.4 Add Go client cases using `GatewayOptions` to the request goldens and `go-client-differential.test.ts`, matching the pinned client's equivalent `GatewayProviderOptions` bodies in unary and streaming modes. Verify with `mise run test-providerwire-v4`.

## 8. Two-client command evidence

- [x] 8.1 Extend `ai-gateway/test/providerwire-v4/gateway-command.test.ts` and its testserver: configured-only, request-only and combined routing for configured (self-hosted) and cloud BYOK access, unary and streaming, through the pinned TypeScript client and the Go client. The Go side uses `GatewayOptions` and reads results and errors with the metadata readers. Assert attempt order, native requests, routing metadata and each diagnostic document. Verify with `mise run test-ai-gateway-command`.
- [x] 8.2 Add cross-population cases (a BYOK caller naming configured routes, a configured caller sending `byok`) and a Go consumer-middleware case that observes routing metadata with server capture disabled. Verify with the same command and the service tests.

## 9. Documentation

- [x] 9.1 Document routing controls, slug identity, precedence, diagnostics, `providerTimeouts.byok`, `gateway.routing` and the support boundary in `docs/providers/grafana-gateway.md`. Use `GatewayOptions` for Go (replacing the raw BYOK example), `GatewayProviderOptions` for TypeScript, and the metadata readers in the execution-overview section. Verify that the examples match the command test cases and that the Go snippets compile as an `example_test.go` in `providers/grafana`.
- [x] 9.2 Update `docs/concepts/gateway-architecture.md` (selection and composition flow, diagram) and `docs/concepts/gateway-glossary.md` (routing controls, model group, composed chain, provider slug, provider start timeout). Verify the Mermaid diagrams render and the descriptions match the code.

## 10. Integration checks

- [ ] 10.1 Run `mise run fmt`, `mise run lint`, `mise run test`, `mise run test-providerwire-v4`, `mise run test-ai-gateway-command`, the plugin validation and `mise run parity-check`, and verify that all pass with no conformance fixture or `upstream.yaml` changes.
- [x] 10.2 Run `openspec validate add-gateway-request-routing --strict` and verify that it passes.

## Workflow follow-up

- Open one draft PR, `feat!: support Vercel-compatible request routing in the Gateway and Grafana client`, against `main`. Add a `BREAKING CHANGE:` note in the description for the unsupported-control message change and the change of category for configured callers' controls. Release attribution follows the touched files, so this gives both `providers/grafana` and the Gateway plugin a major release. Report pinned-client compatibility evidence separately from Vercel service design references.
- Before opening it, confirm `mise run release-preview` attributes the change to `providers/grafana` and the Gateway plugin.
- Archive the change after review.
