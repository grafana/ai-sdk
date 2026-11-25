# Tasks

## 1. Remove the egress policy

- [x] 1.1 Delete `outbound/egress.go`, `egress_client.go` and their tests, `ModelClients` and `EgressModel`, and revert `outbound.go` and `outbound_test.go` to `main`; verify with `go build ./...` and `mise run tidy` (`golang.org/x/net` back to indirect).
- [x] 1.2 Delete the egress sentinels and `CategoryEgressRefused` from `execution`, their ProviderWire document and Chat Completions mapping, and their tests; verify with `go test ./internal/... ./providerwire/... ./openai/...`.

## 2. The endpoint list

- [x] 2.1 Add `outbound.CustomEndpoints`, `ParseCustomEndpoints` and `Check`; verify with the parse and match tables in the design.
- [x] 2.2 Replace the allow and deny settings with `custom_endpoints` in `config` and `provisioning/grafana.ini`; verify with settings tests for valid, invalid and removed keys.

## 3. Stack catalogs

- [x] 3.1 `stackcatalog.Parse` and `Validator` take the list. Unlisted endpoints get the field-path message, and the `https`-only message goes away. Verify with the `Parse` and `Validator` tables.
- [x] 3.2 `StackCatalogOptions{Client, Endpoints}` and `StackCatalogLister{Endpoints}`: one client, and a request-time check through `parseStackCatalog`. Verify with `service` tests for a listed endpoint on the shared client and a removed entry as `ErrCatalogConfiguration` with zero builds.

## 4. BYOK

- [x] 4.1 `DecodeRequest(selector, controls, endpoints)`, `byok.New(request, placements, client)` and `BYOKOptions{Client, Endpoints, Factory}`; verify with the decode table and recording-client tests.
- [x] 4.2 Wire the list in `gateway.newRouteHandler` and `managedKinds`. Verify with cloud-mode harness tests: stack-catalog and BYOK calls to a listed loopback TLS fake, unary and stream; unlisted endpoints as 400 (admission and BYOK) with zero requests; and the operator catalog with an empty list.

## 5. Cross-language and e2e evidence

- [x] 5.1 Replace the `test/providerwire-v4` egress scenarios with an unlisted BYOK endpoint scenario through both clients, and an admission refusal for a stack catalog; verify with `mise run test-providerwire-v4`.
- [x] 5.2 Replace `tests/catalog-egress.spec.ts` with `tests/catalog-endpoints.spec.ts` and update `catalog.spec.ts`. Set `GF_PLUGIN_GRAFANA_AIGATEWAY_APP_CUSTOM_ENDPOINTS` in `docker-compose.e2e.yaml`, and drop the aliases, their SANs and the per-host counter. Verify with prettier, eslint, a tsc run that includes the tests, `bash -n` and shellcheck on `e2e.sh`, and CI's Playwright job (26 tests).

## 6. Documentation

- [x] 6.1 Rewrite the README custom-endpoint section and the settings table, including the security notes (list only endpoints you control or vendors whose DNS you trust; in `cloud` mode any user or service account can use a listed endpoint through BYOK; listed `http` sends keys in cleartext). Update the BYOK client guide, the glossary entry and the architecture sentence. Verify with `mise run lint-docs`.

## 7. Integration checks

- [x] 7.1 Run `mise run check`, `lint`, `build`, `test-ai-gateway`, `test-providerwire-v4`, `test-integration`, `verify-ai-gateway-boundary`, `verify-module-requirements`, `release-check` and `lint-docs`, and `-race` on `outbound`, `byok`, `stackcatalog`, `service`, `gateway` and `openai/...`, with `MISE_GO_VERSION=1.27.3` and `GOFLAGS=-buildvcs=false`. OpenSpec validation runs in CI's Validate Changes.

## Workflow follow-up

- Archive `add-custom-provider-endpoints` as the last commit, so #778 ends with exactly one archived change.
