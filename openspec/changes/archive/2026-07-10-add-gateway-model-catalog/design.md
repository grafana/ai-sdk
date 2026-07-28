## Context

The repository currently has three related but separate layers:

- `registry.ProviderRegistry` resolves composite `provider:model` IDs into provider models and applies registry middleware.
- `provider/wire` defines the Go-to-Go HTTP and SSE transport for model calls.
- `providers/grafana` is a remote gateway-style client that forwards any supplied model ID.

The Assistant service adds a fourth concern in `api/internal/aisdkplatform.Catalog`: a finite map of flat public IDs to fully composed models, canonical public-ID resolution, sorted listing, and unknown-model handling. Its primary generic use case is exposing one model-family/version ID such as `claude-opus-4-8` while the composed model can invoke Anthropic, Vertex, Bedrock, or a fallback across them. That catalog also contains Assistant-specific profiles and Claude normalization that must not become ai-sdk policy.

A concurrent `extract-provider-wire-server` change proposes `gateway/providerwire` as the reusable HTTP execution surface. It defines an HTTP-facing `ModelResolver` that receives `*http.Request` and returns only `provider.LanguageModel`. That boundary is adjacent to, but intentionally different from, this change's transport-neutral catalog resolver, which receives `context.Context` and also returns canonical public identity.

The registered upstream baseline pins `ai@7.0.19`; that package transitively resolves the lockfile-pinned `@ai-sdk/gateway@4.0.15`. The matching local upstream release commit, resolved from both package version changes, is `405116e4773267ab34ad6bfb20015ce8b49b8db3`. At that reference, provider registry routing remains separate from gateway model discovery. The Go design should preserve that responsibility boundary rather than adding a second namespace mode to the registry.

## Goals / Non-Goals

**Goals:**

- Define request-aware contracts for resolving and listing public gateway models.
- Provide immutable static and registry-backed catalog implementations.
- Preserve canonical public identity independently of model-reported, provider-specific identity.
- Support explicit aliases, minimal listing metadata, typed capabilities, and machine-identifiable unknown-model errors.
- Make request-scoped policy possible through interface decorators without implementing application policy.
- Compose cleanly with `gateway/providerwire` through a host-owned adapter while allowing either package to land independently.
- Keep the implementation transport-neutral and free of new external dependencies.

**Non-Goals:**

- Change `registry.Provider`, `ProviderRegistry`, `CustomProvider`, or composite-ID semantics.
- Add an HTTP model-listing endpoint or change provider-wire framing and payloads.
- Import `net/http` or `gateway/providerwire`, implement its HTTP resolver, or make either package depend on the other.
- Add provider credentials, provider discovery, provider ordering, fallback construction, or middleware composition.
- Add Assistant model families, profile slots, `chat-large`/`chat-small`, Claude aliases, entitlements, or feature flags.
- Define pricing, lifecycle, model-type, or a universal capability vocabulary.
- Infer public identity from `LanguageModel.ModelID()` or provider `ModelIDs()` lists, translate provider-specific IDs, or ship built-in public model names.
- Migrate the Assistant repository as part of this change.

## Decisions

### 1. Add a focused `gateway/catalog` package

The parent `gateway` directory is a namespace for independent gateway surfaces. The `gateway/catalog` package owns the public model catalog, while the separately proposed `gateway/providerwire` package owns HTTP execution. The catalog will not live in `registry`, because registry IDs identify construction providers and require a separator. It will not live in `provider/wire`, because the catalog is useful in-process and does not define bytes or HTTP routes. It will not live in `providers/grafana`, which is a remote client rather than a server-side namespace.

Alternative considered: place the catalog in a top-level `gateway` package. Rejected because every current symbol is catalog-specific, the planned provider-wire API is an independent sibling, and a broad root package would obscure ownership as more gateway surfaces are added. Alternative considered: use only `registry.CustomProvider`. That is sufficient for flat string-to-model lookup, but it does not return canonical identity, model listing, alias metadata, request-aware contracts, or public-specific unknown-model errors. Alternative considered: extend `ProviderRegistry` with flat-ID and listing modes. Rejected because it would create ambiguous ID semantics, add request and metadata concerns to provider construction, and change an upstream-aligned public contract.

### 2. Separate resolution and listing interfaces

The public contracts will be equivalent to:

```go
type ModelResolver interface {
    ResolveModel(ctx context.Context, modelID string) (ResolvedModel, error)
}

type ModelLister interface {
    ListModels(ctx context.Context) ([]ModelInfo, error)
}

type Catalog interface {
    ModelResolver
    ModelLister
}
```

Both methods receive `context.Context` so a consumer-owned decorator can apply the same request-scoped visibility policy to lookup and discovery. Static implementations do not retain the context. Separate interfaces let handlers depend only on resolution while discovery surfaces require listing.

Alternative considered: pass `*http.Request` or an Assistant caller type. Rejected to keep the package transport- and application-neutral. The separately proposed `gateway/providerwire.ModelResolver` deliberately accepts `*http.Request` because it is an HTTP execution seam; a host adapter bridges that qualified interface to `catalog.ModelResolver` rather than making the contracts directly assignable.

### 3. Return canonical public identity with the model

`ResolvedModel` will contain the canonical public `ID` and the resolved `provider.LanguageModel`. The public ID is not derived from `LanguageModel.ModelID()`: a public route can resolve to a model with a provider-specific invocation identity or to a fallback whose backend changes per call.

Alternative considered: return only `provider.LanguageModel`. Rejected because callers need stable public identity for profiles, logs, policy, and future gateway responses.

### 4. Use explicit immutable entries and aliases

`catalog.NewStatic` will accept entries containing `ModelInfo` and a non-nil model. `catalog.NewRegistry` will accept route entries containing `ModelInfo` and an opaque provider model ID. Constructors will copy input data and reject empty canonical IDs, empty aliases, empty registry provider model IDs, nil models/providers, duplicate canonical IDs, duplicate aliases, and aliases colliding with any canonical ID.

Aliases will be declared on the canonical entry. Resolution by an alias returns the canonical ID, and listing returns one canonical row with aliases as metadata. No normalization callback will be offered because arbitrary normalization makes collision validation and listing inconsistent. Assistant can convert its existing Claude alias table into explicit aliases.

Alternative considered: list aliases as independent models. Rejected because it obscures canonical identity and inflates discovery results.

### 5. Keep listing metadata minimal and transport-neutral

`ModelInfo` will contain:

- required canonical `ID`;
- optional `Name` and `Description` presentation fields;
- explicit `Aliases`;
- `Capabilities []ModelCapability`.

The structs will not define a future HTTP response schema, arbitrary metadata map, pricing, provider-specific invocation identity, or JSON compatibility promise. Listing is sorted by canonical ID and returns defensive copies of slice fields.

Capabilities describe the guaranteed public route and are supplied by the catalog owner. For a fallback route, callers must advertise only capabilities shared by every possible backend. `ModelCapability` is a named string type, but this change will not invent built-in constants before gateway capability semantics are defined.

Alternative considered: derive public identity, metadata, or capabilities from model/provider names or `LanguageModel.ModelID()`. Rejected because middleware can override the reported model ID, fallback models can execute a different candidate, remote routes can report routing identities, and provider-specific invocation IDs can expose backend details.

### 6. Provide a structured unknown-model error without enumerating the catalog

The package will expose `ErrUnknownModel` and a pointer `*UnknownModelError` containing the requested public ID and unwrapping to the sentinel. `Error` and `Unwrap` will use pointer receivers. Callers can use `errors.Is(err, ErrUnknownModel)` or declare `var target *catalog.UnknownModelError` and call `errors.As(err, &target)`. The error will not include available IDs, because request-filtered catalogs must not disclose inaccessible models and large catalogs make enumeration unbounded.

This narrowly scoped typed error is an intentional exception to the repository convention of sentinels plus `fmt.Errorf` wrapping because the issue requires machine-readable requested-ID inspection at a gateway boundary. Other validation and downstream failures continue to follow the normal sentinel-wrapping or contextual `fmt.Errorf` convention.

Alternative considered: return `provider.APICallError`. Rejected because an absent local public route is not a provider API call. At an HTTP composition edge, the host adapter must map `ErrUnknownModel` to a non-retryable 404 `*provider.APICallError` whose cause is the original catalog error; this preserves `errors.Is` and `errors.As` in-process without coupling the catalog package to HTTP.

### 7. Adapt `registry.Provider` through explicit routes

The registry-backed catalog will accept the narrow `registry.Provider` interface, not concrete `*ProviderRegistry`. Each route explicitly maps a public `ModelInfo.ID` to the opaque string passed unchanged to `Provider.LanguageModel`; that string may be a composite registry ID, a custom-provider alias, or another provider-specific ID.

The adapter will resolve models on demand, return the public canonical ID, and preserve provider/registry errors. Only absence of the public route or alias produces `UnknownModelError`. A nil model returned without an error is treated as an invalid provider result, not an unknown public route.

Alternative considered: automatically derive public IDs from registry provider/model IDs. Rejected because canonical public model-family/version IDs and provider-specific invocation IDs are separate namespaces and derivation would leak routing structure.

### 8. Treat this as a local API addition, not a parity wire change

The package only selects existing `provider.LanguageModel` values. It does not change provider options, requests, stream parts, UI chunks, SSE, or provider-wire bytes. Unit tests and `validate-parity-baseline` are required, but baseline validation only checks registered TypeScript consumers; it does not prove gateway architecture alignment. Implementation review must separately compare the provider registry and gateway metadata separation against registered `ai@7.0.19` and its lockfile-pinned transitive `@ai-sdk/gateway@4.0.15` source and tests. Any later listing endpoint or change to forwarded model identity must add the appropriate conformance coverage and decide whether to register gateway as direct parity scope.

### 9. Compose with the provider-wire server through a host adapter

The `extract-provider-wire-server` plan defines `providerwire.ModelResolver.ResolveLanguageModel(*http.Request, string) (provider.LanguageModel, error)`. This change defines `catalog.ModelResolver.ResolveModel(context.Context, string) (ResolvedModel, error)`. The duplicate short name is acceptable because the package-qualified contracts represent different layers and have intentionally different results.

Neither package will import the other. A host-owned `providerwire.ModelResolverFunc` adapter will perform any authenticated HTTP policy, call the catalog with `r.Context()`, return `ResolvedModel.Model` for execution, and retain `ResolvedModel.ID` only where host logging or policy needs canonical identity. If catalog resolution matches `catalog.ErrUnknownModel`, the adapter must return a non-retryable HTTP 404 `*provider.APICallError` with the catalog error as its cause. Other catalog or registry failures pass through unchanged for the provider-wire handler's documented normalization.

This mapping is required because `gateway/providerwire` intentionally preserves `*provider.APICallError` but normalizes arbitrary resolver errors to retryable HTTP 502. Passing `*catalog.UnknownModelError` through directly would regress Assistant's current unknown-model behavior from non-retryable 404 to retryable 502. The catalog package will not add an HTTP helper; composition remains host-owned and transport-neutral.

Alternative considered: make one resolver implement or import the other. Rejected because it creates landing-order/API coupling, mixes HTTP request lifetime with catalog policy, and constrains future dependency direction. Alternative considered: require a direct cross-package integration test in this change. Rejected because documentation and later Assistant migration tests can prove the adapter without making either additive package depend on the other's landing order.

## Risks / Trade-offs

- [Public API over-generalization] → Keep metadata finite and omit pricing, arbitrary maps, built-in capability policy, mutation, and HTTP DTOs.
- [Alias collision or unstable canonicalization] → Validate the complete canonical and alias namespace during construction and use exact string matching.
- [Resolution/listing policy mismatch] → Make both operations context-aware and document that policy decorators must apply one visibility rule to both.
- [Model disclosure through errors] → Return only the requested ID in unknown-model errors; use the listing contract for discovery.
- [Capability drift across fallback providers] → Require catalog owners to declare the guaranteed intersection and avoid inference.
- [Registry construction failure mistaken for public absence] → Preserve downstream provider errors and generate `ErrUnknownModel` only before registry delegation.
- [Package-name confusion with remote gateway clients] → Document `gateway/catalog` as the public model catalog, `gateway/providerwire` as HTTP server execution, and `providers/grafana` as the remote client.
- [Two qualified `ModelResolver` contracts are mistaken as interchangeable] → Document their signatures and host adapter explicitly; do not alias them or add a cross-package dependency.

## Migration Plan

1. Add the new package, contracts, static catalog, registry adapter, tests, godoc, and guide without changing existing consumers or depending on `gateway/providerwire`.
2. Validate root tests, lint/vet checks, examples, and parity baseline metadata.
3. Allow either additive package to land first: `gateway/providerwire` can initially wrap the existing Assistant catalog, while `gateway/catalog` can initially remain behind the existing Assistant handler.
4. In a separate Assistant change, construct generic entries from existing composed models, move profiles to an Assistant-owned layer, and bridge the two resolver contracts with a host-owned adapter that passes `r.Context()` and maps `catalog.ErrUnknownModel` to non-retryable HTTP 404.
5. Before switching Assistant resolution, run side-by-side tests that preserve unknown-model 404 behavior as well as existing successful model dispatch.
6. If migration exposes a behavior mismatch, roll back the Assistant integration; neither additive ai-sdk package changes an existing runtime path or imports the other.

## Open Questions

None block this change. A future gateway API proposal must separately define capability constants, HTTP listing DTOs, pricing/lifecycle fields, and conformance coverage before adding them to this package.
