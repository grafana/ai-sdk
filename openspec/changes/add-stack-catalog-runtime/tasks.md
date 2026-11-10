# Tasks

## 1. Optional operator catalog

- [x] 1.1 Make `model_catalog` optional in `config.Settings.Validate`, keeping the 64 KiB bound; verify settings tests for unset, blank, whitespace and oversized values.
- [x] 1.2 In `gateway.New`, skip parsing, endpoint validation and secret resolution for a blank catalog and build an empty `service.Catalog`. Verify with gateway tests: startup without the setting, `config` returns `{"models":[]}`, a configured call (plain and routed) returns the model-not-found document with zero provider requests, and a `cloud`-mode BYOK call succeeds.
- [x] 1.3 Update the `ai-gateway/README.md` settings table (`model_catalog` optional, and what the namespace routes serve without it); verify `mise run lint-docs`.

## 2. Execution: source, redaction and category

- [x] 2.1 Add `SourceStackCatalog`, `Selection.Catalog`, the unexported request keys and `StackCatalogSelection` (configured attribution); verify with execution unit tests for source, attribution, name and cloned keys.
- [x] 2.2 Add `SummaryRedactor.With` and build each call's capture with the selection's keys. Verify with runtime tests: the overview and `View.Current` redact catalog, unused-provider and startup keys, unary and stream; a later call is unaffected.
- [x] 2.3 Add `ErrCatalogConfiguration` and `CategoryCatalogConfiguration` in `ClassifySelection`; verify with the classification table test.
- [x] 2.4 Add the fixed ProviderWire document (424, `failed_dependency`, fixed message, not retryable). Verify with document tests for unary and stream setup. The Go client (`TestGatewayError_RegisteredMatrix`) and TypeScript (`client-consumption.test.ts`) matrices already map 424 `failed_dependency`/`failed_dependency` as not retryable, whatever the message.

## 3. Stack-catalog selection

- [x] 3.1 Extract per-candidate construction from `buildCatalog` and the routed composition from `selectRouted` into shared helpers; verify every existing service test passes unchanged.
- [x] 3.2 Add `StackCatalog`, `StackCatalogOptions` and `SelectStackCatalog`, with the check order from the design and an injectable constructor. Verify with table tests: build counts for plain, alias, `models`, `only`, `order` and a 50-model catalog; attempt bound with zero builds; unknown model and `models` entry; `byok` and unknown controls refused with zero builds; invalid spec, and missing or empty key, returning the bare sentinel with no spec text.
- [ ] 3.3 Verify fallback and overview: the primary fails, the fallback succeeds, the overview lists both attempts with catalog provider names, and no physical attempt record is written.

## 4. Stack-catalog observation

- [ ] 4.1 Add the catalog field to `requestObservation` and the per-call binding, emit `catalog` in logs and `gateway.catalog` in Agent Observability metadata, and extend the log and metadata allowlists. Verify with observation tests for the bound and for omission on non-catalog calls.
- [ ] 4.2 Add the `stack-catalog` chain to `ModelObservers` (metric identity view under the shared middleware); verify that `Compose` accepts the source and that the unknown-source test still fails as internal.
- [ ] 4.3 Add the composition test with production `ModelObservers`, `SelectStackCatalog` and the runtime. Verify, unary and stream:
  - each observer runs once;
  - log and Agent Observability identity and metadata are as specified;
  - the metric model label is `catalog`, and 20 catalogs × 20 model IDs give one series set;
  - no key, provider name, alias or backend ID appears in any sink.

## 5. Documentation

- [ ] 5.1 Add stack-catalog limits to the README Limits ledger. In `ai-gateway/docs/text-observability.md`, document the `catalog` model label (and its overlap with an operator model named `catalog`), the `catalog` log field and the `gateway.catalog` export metadata. Verify `mise run lint-docs`.
- [ ] 5.2 Update `docs/concepts/gateway-architecture.md` (the `stack-catalog` source, per-request build, third chain) and the glossary (stack-catalog selection, catalog configuration error); verify `mise run lint-docs`. Mermaid rendering is checked on GitHub.

## 6. Integration checks

- [ ] 6.1 Run `mise run fmt-check`, `vet`, `lint`, `build`, `test-ai-gateway`, `test-integration`, `verify-ai-gateway-boundary` and `verify-module-requirements` with `GOFLAGS=-buildvcs=false`; verify all pass.
- [ ] 6.2 Run `openspec validate add-stack-catalog-runtime --strict`; verify it passes.

## Workflow follow-up

- CI runs the Playwright end-to-end suite; there is no local docker.
- Archive the change after review.
