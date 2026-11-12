# Tasks

## 1. Shared endpoint rule in `outbound`

- [x] 1.1 Move the per-URL endpoint check from `stackcatalog.checkEndpoints` to `outbound.CheckEndpoint` with exported sentinels, and keep `stackcatalog`'s field-path messages byte-identical; verify with a `CheckEndpoint` table and the unchanged `TestParse_RefusesEndpoints`.
- [x] 1.2 Move `service.egressCandidate` to `outbound.EgressModel`, and add `outbound.ModelClients` with `For(baseURL)`; verify with the moved bare-error test over the three provider types and a `For` table.
- [x] 1.3 Switch `StackCatalogOptions` to `Clients outbound.ModelClients` and use `For` and `EgressModel` in `SelectStackCatalog`; verify with the existing stack-catalog service and gateway tests.
- [x] 1.4 Count a `baseURL` equal to its type's native default as unset in `CheckEndpoint`, with `nativemodel.DefaultBaseURL`, for stack catalogs; verify with a `CheckEndpoint` table, a stack `Parse` case and a deny entry for the native host that doesn't refuse it.

## 2. Settings

- [x] 2.1 Rename the settings to `custom_endpoint_allow` and `custom_endpoint_deny` in `config`, `provisioning/grafana.ini` and `docker-compose.e2e.yaml`; verify with the settings tests, `bash -n scripts/e2e.sh` and a YAML parse.

## 3. BYOK

- [x] 3.1 Replace `DecodeRequest`'s `approvedBaseURLs` with the egress policy, check every account `baseURL` with `outbound.CheckEndpoint` and store the normalized URL; verify with a decode table (public, native, `http`, userinfo, refused literals, numeric host, deny hostname, unused provider) asserting `ErrInvalidRequest` with no URL in the message.
- [x] 3.2 Give `byok.New` the clients, choose each account's client with `For` and wrap custom endpoints in `EgressModel` inside `Translate`; verify with recording clients for client choice and a refused-then-working account chain.
- [x] 3.3 Give `BYOKOptions` the clients and policy and wire them in `gateway.newRouteHandler`; verify with cloud-mode harness tests: a loopback TLS fake through an allow entry (unary and stream), a refused literal as the 400 BYOK document, a refused loopback name as the 424 egress document with zero requests, and an overview without URLs or addresses.

## 4. Cross-language evidence

- [x] 4.1 Add a `test/providerwire-v4` scenario in which both clients send BYOK accounts with a refused literal (400) and a refused loopback name (424, not retryable, zero connections); verify with `mise run test-providerwire-v4`.

## 5. Documentation

- [x] 5.1 Update the README (the custom-endpoint section covers request accounts, the widened set of callers and the proxy limit; the settings table names the new keys), the client guide's BYOK section (`baseURL` and the errors), the glossary's BYOK and egress entries, the architecture walkthrough sentence, and the `gateway-stack-catalog-egress` Purpose; verify with `mise run lint-docs`.

## 6. Integration checks

- [ ] 6.1 Run `mise run fmt-check`, `vet`, `lint`, `build`, `test`, `test-ai-gateway`, `test-providerwire-v4`, `test-integration`, `verify-ai-gateway-boundary`, `verify-module-requirements`, `release-check`, `lint-docs` and `openspec validate byok-custom-endpoints --strict`, plus `-race` on `outbound`, `stackcatalog`, `byok`, `service` and `gateway`, all with `GOFLAGS=-buildvcs=false`.

## Workflow follow-up

- Merge `nrbrd/758-stack-catalog-routes` when the #771 refactor lands in #775, before implementation if possible.
- Archive the change as the last commit after review.
