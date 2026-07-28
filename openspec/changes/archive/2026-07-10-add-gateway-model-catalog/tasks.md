## 1. Public Gateway Contracts

- [x] 1.1 Add the transport-neutral `gateway/catalog` package with `ModelResolver`, `ModelLister`, `Catalog`, `ResolvedModel`, `ModelInfo`, and `ModelCapability`, including exported symbol documentation, compile-time interface checks, and no `net/http` or `gateway/providerwire` dependency.
- [x] 1.2 Add `ErrUnknownModel` and pointer `*UnknownModelError` with requested-ID access, sentinel unwrapping, non-enumerating error text, and explicit `errors.Is`/pointer `errors.As` tests.
- [x] 1.3 Add entry and route types plus shared validation and defensive-copy helpers for canonical IDs, aliases, capabilities, models, and providers.

## 2. Static Catalog

- [x] 2.1 Add table-driven tests for static canonical and alias resolution, public versus model-reported provider identity, stable listing, empty catalogs, defensive copying, unknown-model errors, and every namespace validation failure.
- [x] 2.2 Implement `catalog.NewStatic`, `ResolveModel`, and `ListModels` to satisfy the static catalog tests without retaining request contexts or exposing mutation.

## 3. Registry Provider Adapter

- [x] 3.1 Add tests using hand-written `registry.Provider` implementations for exact opaque-ID delegation, canonical alias results, custom/nested provider compatibility, stable metadata listing, empty route-target rejection, downstream error preservation, nil-model rejection, and unknown public routes.
- [x] 3.2 Implement `catalog.NewRegistry`, `ResolveModel`, and `ListModels` over the narrow `registry.Provider` interface while keeping registry and transport behavior unchanged.

## 4. Documentation

- [x] 4.1 Complete package godoc describing `gateway/catalog` as the public model catalog, the distinct package-qualified `catalog.ModelResolver` contract, request-aware policy decorators, canonical identity, alias and capability semantics, registry adaptation, and explicit non-goals.
- [x] 4.2 Add a focused gateway model catalog guide, link it from `docs/README.md`, and update the fallback/registry guide to distinguish provider construction from public gateway catalogs without duplicating API reference details. Always document the generic host-adapter semantics: pass request context, return `ResolvedModel.Model`, retain the canonical ID for host concerns, map `ErrUnknownModel` to a non-retryable 404 `*provider.APICallError` with the catalog error as cause, and pass other failures through unchanged. Name and link the concrete `gateway/providerwire.ModelResolver` API only if that package exists in the implementation branch; otherwise leave the concrete cross-link to the later provider-wire or Assistant migration change.

## 5. Validation

- [x] 5.1 Run `mise run fmt`, focused gateway and registry tests, `mise run vet`, `mise run lint`, `mise run test`, and `mise run build`; resolve all failures introduced by the change.
- [x] 5.2 Compare registry/catalog responsibility boundaries against registered `ai@7.0.19`, its lockfile-pinned transitive `@ai-sdk/gateway@4.0.15`, and the concurrent `extract-provider-wire-server` design; record the resolved upstream reference, run `mise run validate-parity-baseline`, verify the catalog does not import `net/http`, `gateway/providerwire`, or concrete provider model inventories, and confirm the diff does not change provider requests, provider-wire serialization, stream/UI chunks, SSE framing, or the existing registry contract.
- [x] 5.3 Run `openspec validate add-gateway-model-catalog` and verify every specification scenario has corresponding test or documentation evidence before marking the change complete.
