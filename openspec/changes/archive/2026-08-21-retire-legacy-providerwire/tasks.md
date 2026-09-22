## 1. Confirm Retirement Preconditions

- [x] 1.1 Confirm whether any known external consumer imports `github.com/grafana/ai-sdk/providers/grafana` and requires an independently installable rollback; if one exists, stop and plan a validated nested-module release before deletion.
- [x] 1.2 Record that root `github.com/grafana/ai-sdk@v0.1.0-alpha.1` is the rollback point only for root-module/server deployments and that Grafana-client source remains available at repository tag `v0.1.0-alpha.1` or in Git history without a published nested-module claim.

## 2. Remove Legacy Production Surfaces

- [x] 2.1 Delete the complete `gateway/providerwire` package without aliases, forwarding shims, copied codecs, or a replacement transport.
- [x] 2.2 Delete the complete `providers/grafana` module, including its client, authentication helpers, options, gateway-error normalization, tests, and module dependency files.
- [x] 2.3 Verify no remaining production package imports or re-exports either retired path and that `provider`, concrete providers, `gateway/catalog`, fallback, registry, and middleware remain available.

## 3. Remove Legacy-Only Verification

- [x] 3.1 Delete the `test/interop` TypeScript workspace and Go test server that exercise the retired public handler.
- [x] 3.2 Delete `test/conformance/grafana` and remove the Grafana module requirement and replacement from the conformance module.
- [x] 3.3 Tidy the conformance module and verify Anthropic, Bedrock, OpenAI, OpenAI-compatible, provider-independent UI, and `test/integration` coverage remain registered.

## 4. Clean Repository Registration

- [x] 4.1 Remove `providers/grafana` and `test/interop/testserver` from `go.work`, then synchronize the workspace dependency metadata.
- [x] 4.2 Remove Grafana test/build/vet/tidy commands and the legacy interop task from `mise.toml`, including aggregate task references.
- [x] 4.3 Remove the interop dependency source and pnpm workspace entry, regenerate `test/pnpm-lock.yaml` without the deleted importer, and retain exact registered packages still consumed elsewhere.
- [x] 4.4 Update baseline validation and upgrade scripts so they no longer enumerate the deleted interop package as a parity consumer.
- [x] 4.5 Remove the dedicated legacy interop CI job and update contributor documentation that lists the retired harness or command.

## 5. Reframe Retained Provider JSON Behavior

- [x] 5.1 Preserve existing provider-domain JSON behavior and tests unchanged while rewording only current comments or godoc that present provider structs or marshalers as ProviderWire HTTP authority.
- [x] 5.2 Remove the dedicated Grafana transport and ProviderWire encode-compatibility rows from `test/conformance/PARITY.md` without adding a replacement transport classification.

## 6. Update Documentation and Active Contracts

- [x] 6.1 Delete `docs/guides/provider-wire-server.md` and `docs/providers/grafana-cloud.md`, then remove their index entries, cross-links, and adjacent-page navigation references.
- [x] 6.2 Revise `docs/guides/gateway-model-catalog.md` to remain transport-independent and remove examples or claims tied to `providerwire.Handler`.
- [x] 6.3 Remove retired-provider references from installation, provider overview, middleware, and UI wire-protocol documentation without removing supported catalog, middleware, provider, or UI SSE guidance.
- [x] 6.4 Apply the retirement deltas for `provider-wire`, `gateway-providerwire-server`, `grafana-provider`, `grafana-provider-options`, `gateway-error-normalization`, `conformance-testing`, `upstream-parity-governance`, `api-call-error`, `docs-structure`, `gateway-model-catalog`, `prometheus-middleware`, and `context-enrichment-middleware` so no active specification requires deleted code or tests.

## 7. Verify Repository Integrity

- [x] 7.1 Search tracked production, test, tooling, CI, parity, documentation, and `openspec/specs/` files for stale normative references to the two import paths, deleted interop package/task, Grafana conformance, and removed docs; only archived OpenSpec changes and explicitly historical retirement/source references may remain.
- [x] 7.2 Run `mise run lint-docs`, `mise run validate-parity-baseline`, and `mise run parity-check` without regenerating or inventing conformance fixtures.
- [x] 7.3 Run `mise run test-integration` to validate the retained TypeScript workspace, lockfile, Go test server, and frontend hook coverage.
- [x] 7.4 Run `mise run build`, `mise run vet`, `mise run lint`, `mise run test`, and `mise run verify-module-resolution` across every retained or published module.
- [x] 7.6 Run `openspec validate retire-legacy-providerwire --strict` and confirm the final diff contains only retirement, registration, documentation, parity, provider-domain wording, dependency, and OpenSpec changes.
