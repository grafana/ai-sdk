# Tasks

## 1. Manifest and role

- [ ] 1.1 Add the two catalog `openapi` paths with their declared `get` checks on the route subresources, and the `aigateway:modelinvoker` role bound to Editor. Regenerate the manifest and types with `pnpm --filter ./ai-gateway generate:kinds`. Verify that the manifest tests pin the four routes, the checks, the role's three kinds and the binding, and that `simple.App.ValidateManifest` passes.

## 2. Access, parent check and route table

- [ ] 2.1 Add `CatalogAccounts` and `AccessPolicy.DecideCatalog`; verify a table test over user, service account, access policy alone, access policy for a user, anonymous and namespace mismatch in both modes, with the `Decide` tests unchanged.
- [ ] 2.2 Add the `parentCatalog` middleware and the private stack-catalog context carrier; verify tests for a missing parent, mismatched plural, name, namespace or group/kind, an undecodable parent, and that the body isn't read.
- [ ] 2.3 Add the two route-table entries with their labels, `DecideCatalog` and `parentCatalog`; verify the route-table and manifest agreement tests, metrics `decision="catalog"` and both labels.

## 3. Invocation and discovery

- [ ] 3.1 Add the `catalog` branch to `NewSelector`, calling `SelectStackCatalog` with the carrier; verify service tests (missing carrier refused; an operator-only model not found; `byok` refused).
- [ ] 3.2 Add `StackCatalogLister` and the catalog-route discovery handler; verify the rows, the error mapping and zero constructions.
- [ ] 3.3 Add `host.ErrorCatalogConfiguration`, its discovery mapping and its ProviderWire document; verify the document bytes match the runtime 424 document.

## 4. Test host and protocol tests

- [ ] 4.1 Serve declared subresource routes in `platformhost` with the stored object, resource version and decrypted values as the parent, and `404` for a missing object; verify host tests.
- [ ] 4.2 Add gateway harness tests through the test host: invocation with the catalog key reaching a fake native provider, unary and stream, alias and fallback, an edit applying on the next call, discovery, refusals, an access-policy token (alone and for a user), and no key in logs or metrics.

## 5. Cross-language scenarios

- [ ] 5.1 Add the catalog scenarios to `ai-gateway/test/providerwire-v4`. Through `plugin-host --manifest` and the native proxy, both registered clients list `config` and make unary and streamed calls with the catalog base URL, and the fake provider sees the catalog key. Add Kubernetes `Status` `403`/`404` consumption by both clients. Verify with `mise run test-providerwire-v4`.

## 6. End-to-end

- [ ] 6.1 Make `startNativeProviderProxy` take its certificate and key, keeping the existing fixture for `test/providerwire-v4`. Add:
  - an e2e `native-proxy` service that runs it with a certificate `scripts/e2e.sh` generates at run time, with nothing committed;
  - the proxy environment in `docker-compose.e2e.yaml`;
  - the key-hash reply in `fake-anthropic.mjs`.

  Verify that `mise run test-providerwire-v4` still passes and that `tsc`, `eslint`, `prettier` and `playwright test --list` pass.
- [ ] 6.2 Add `tests/catalog-routes.spec.ts` with the design's table: roles, decrypted keys, edits, discovery, `403`/`404` and namespace routes unchanged. Verify in CI's Playwright job; there is no local docker.

## 7. Documentation

- [ ] 7.1 README: catalog routes and base URL, the `modelinvoker` role, Grafana's `403`/`404` responses, the one-minute decrypted-value cache, and the access-policy permission. Client guide (`docs/providers/grafana-gateway.md`): the catalog base URL. Verify `mise run lint-docs`.
- [ ] 7.2 Architecture walkthrough and diagrams (the catalog account path) and glossary (catalog route, catalog access, `aigateway:modelinvoker`); verify `mise run lint-docs`. Mermaid rendering is checked on GitHub.

## 8. Integration checks

- [ ] 8.1 Run `mise run fmt`, `vet`, `lint`, `test`, `test-ai-gateway`, `test-integration`, `test-providerwire-v4`, `build`, `verify-ai-gateway-boundary`, `verify-module-requirements`, `release-check` and `lint-docs` with `GOFLAGS=-buildvcs=false`; verify they all pass.
- [ ] 8.2 Run `openspec validate add-stack-catalog-routes --strict`; verify it passes.

## Workflow follow-up

- CI runs the Playwright suite. The 13.3.0 one-off run also covers `catalog-routes.spec.ts`. The access-policy acceptance is verified in Cloud once #460 lands.
- The PR description records the `test/integration/` deviation and the #763 attribution follow-up.
- Archive the change after review.
