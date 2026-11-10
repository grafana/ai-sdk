# Tasks

## 1. Selection source and runtime seam

- [x] 1.1 Add the typed `execution.Source` with `operator-catalog` and `request-byok`, the `Selection.Source` field, and set it in `CatalogSelector`, `ConfiguredSelection` and `AccountSelection`; verify with execution unit tests for each constructor's source.
- [x] 1.2 Add the optional `execution.Config.Compose` (`ModelComposer`) applied in `selectModel` after validation, under the same containment; verify with runtime tests: the wrapped model is invoked, a nil wrapper leaves the selection unchanged, and a composer error, panic or invalid model fails as internal with zero model calls.
- [x] 1.3 Update the execution package doc for the seam and verify `go vet` and existing execution tests pass unchanged.

## 2. Observers at the seam

- [x] 2.1 Add `service.ModelObservers` (built once, chain per source, `Compose` refuses unknown sources) and the exported identity factories, and move `NewModelObservabilityFactories` to a `_test.go` helper built from them; verify every existing service test passes unchanged.
- [x] 2.2 Add the identity test: operator-catalog, alias, routed and BYOK calls through `NewSelector`, `ModelObservers` and the runtime, unary and stream, each observed once with the expected log, metric and Agent Observability identity; verify it passes.
- [x] 2.3 Add the per-request selection double test (two keys, own model per call, operator-catalog chain, no key in sinks; empty source fails as internal with zero model calls); verify it passes.
- [x] 2.4 Wire production in `pkg/internal/gateway.newRouteHandler` (identity factories into the catalog and BYOK, `observers.Compose` into the runtime) with a gateway-level test that each configured and routed call is observed once; verify `mise run test-ai-gateway` passes unchanged.

## 3. Public composition package

- [x] 3.1 Add `ai-gateway/pkg/gateway` with `Options` and `Run`, moving `main.run`'s body there, and a `doc.go` marking the API v0 and experimental; verify `go doc ./pkg/gateway` shows the stability statement.
- [x] 3.2 Reduce `pkg/main.go` to a call of `gateway.Run(gateway.Options{})`; verify the plugin-process tests (lifecycle events, idle, health, stop) pass unchanged.
- [x] 3.3 Add the external-module compile test in `pkg/gateway`; verify it builds the generated module offline and skips under `-short`.

## 4. Documentation

- [x] 4.1 Update `docs/concepts/gateway-architecture.md` (diagram and steps 3 and 4: identity at build time, observers after selection, `gateway.Run` as the composition entrypoint) and add glossary entries for the composition entrypoint and selection source; verify `mise run lint-docs` passes and the Mermaid diagram renders.

## 5. Integration checks

- [x] 5.1 Run `mise run fmt-check`, `vet`, `lint`, `build`, `test-ai-gateway`, `test-integration`, `verify-ai-gateway-boundary`, `verify-module-requirements` and `release-check`; verify all pass.
- [x] 5.2 Run `openspec validate expose-gateway-composition --strict`; verify it passes.
