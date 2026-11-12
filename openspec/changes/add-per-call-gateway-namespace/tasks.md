# Tasks

## 1. Namespace resolution in the Grafana client

- [ ] 1.1 Change `TokenExchangeConfig` handling: `BaseURL` is the Gateway API root and refuses a `namespaces` path segment; `Namespace` is optional and must be a DNS label or `*`. Verify with table cases in `TestTokenExchange_ValidationAndExchange`, including a namespaced base URL and invalid namespaces.
- [ ] 1.2 Add `WithNamespace` and resolve one namespace per call in request preparation: default, equal, conflict, missing, invalid, wildcard route, and any call namespace on service-account or access-token providers, all before I/O. Verify with a table test whose transport and exchanger fail the test on any call, covering `ListModels`, `DoGenerate` and `DoStream`.
- [ ] 1.3 Build the route from the root and the resolved namespace for exchange providers, and keep the configured prefix for the other flows. Verify with request-capture tests that one provider without a configured namespace sends discovery, unary and streaming calls for two namespaces to their own routes with their own exchanged tokens.
- [ ] 1.4 Keep on-behalf-of refusal for the wildcard and for non-exchange flows with the resolved namespace. Verify by extending `TestOnBehalfOf_RefusedBeforeIO` and `TestOnBehalfOf_ExchangesSubjectToken` with per-call namespaces.

## 2. Namespace-scoped exchange sharing

- [ ] 2.1 Key shared exchanges by exchange namespace and subject. Verify with a concurrency test: same subject, two namespaces exchange twice with the right namespace each; same namespace and subject share one exchange; per-caller cancellation still returns early.
- [ ] 2.2 Verify with a test that a caller-supplied HTTP client is used by both the exchange and Gateway requests, and that redirects are refused on the exchange.

## 3. Godoc and examples

- [ ] 3.1 Update `TokenExchangeConfig`, `NewWithTokenExchange`, `WithNamespace` and package godoc: API-root base URL, namespace resolution, per-namespace caching, and audience `aigateway.ext.grafana.app` for Grafana Cloud with the access policy requirement. Update `example_test.go` with a multi-stack example. Verify with `go test ./...` and `go vet ./...` in `providers/grafana`.

## 4. Black-box evidence

- [ ] 4.1 Add a call namespace input to `providers/grafana/internal/capture` and update the exchange inputs in `ai-gateway/test/providerwire-v4/gateway-command.test.ts` to the API-root base URL and the documented audience. Verify the existing exchange cases pass with `mise run test-ai-gateway`.
- [ ] 4.2 Add a black-box case in which one exchange client without a configured namespace runs configured discovery, unary and streaming calls for two namespaces, directly and on behalf of a user, through the protocol test host. Verify it passes and asserts each route's namespace and token namespace.

## 5. Documentation

- [ ] 5.1 Update `docs/guides/gateway-authentication.md`: the inputs an internal service needs, the API-root URL, audience `aigateway.ext.grafana.app` and its access policy requirement, the trusted CA through `HTTPClient`, the multi-stack pattern with `WithNamespace`, and the App Platform example on the new base URL. Update `docs/providers/grafana-gateway.md` where it describes the exchange base URL. Keep deployment-specific names out. Verify with `mise run lint-docs` and by building the Go snippets.
- [ ] 5.2 Check `docs/concepts/gateway-architecture.md` and `docs/concepts/gateway-glossary.md` against the change and update only concrete drift. Verify with `mise run lint-docs`.

## 6. Integration checks

- [ ] 6.1 Run `mise run fmt-check`, `vet`, `lint`, `build`, `test`, `test-ai-gateway` and `build-examples`; verify all pass.
- [ ] 6.2 Run `mise run validate-parity-baseline` to confirm no baseline or fixture drift, and `openspec validate add-per-call-gateway-namespace --strict`; verify both pass.
