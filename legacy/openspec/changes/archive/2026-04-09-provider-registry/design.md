## Context

The Go port has no provider management layer. Callers construct models directly via package constructors (`anthropic.New(key, modelID)`) and pass `provider.LanguageModel` to orchestration functions. The upstream Vercel AI SDK provides `createProviderRegistry` and `customProvider` in `packages/ai/src/registry/` for string-based model addressing, aliasing, and access control. The middleware package already exists and provides `WrapLanguageModel`.

## Goals / Non-Goals

**Goals:**
- Port the upstream provider registry and custom provider to Go, matching behavior and semantics
- Enable string-based composite model addressing (`"anthropic:claude-sonnet-4-6"`)
- Support model aliasing, fallback delegation, and access control via custom provider
- Support optional middleware wrapping at the registry level
- Keep the implementation scoped to language models only (matching our current provider surface)

**Non-Goals:**
- Embedding models, image models, or other model types (add when those interfaces exist)
- Provider version normalization (Go port only has V4, no V2/V3 adapters needed)
- Making the anthropic module implement `Provider` (separate change)
- Global/singleton registry patterns

## Decisions

### 1. Package location: new `registry` package in root module

The `Provider` interface, `ProviderRegistry`, and `CustomProvider` live in a new `registry/` package within the root module. This keeps the registry separate from the leaf `provider/` package (which has zero deps on root) and separate from `middleware/` (single responsibility).

The `registry` package imports `provider` (for `LanguageModel`) and `middleware` (for `WrapLanguageModel`). It does not import the root `aisdk` package.

**Alternative considered**: Putting `Provider` interface in `provider/` package. Rejected because it would create a dependency from `provider` to `middleware` (for registry middleware support), breaking the leaf package constraint.

**Alternative considered**: Putting everything in the root `aisdk` package. Rejected because it would bloat the already-large orchestration package and mix provider management concerns with streaming/SSE.

### 2. Provider interface: single method, error return

```
type Provider interface {
    LanguageModel(modelID string) (provider.LanguageModel, error)
}
```

Go idiomatic adaptation: returns `(model, error)` instead of throwing. The upstream uses synchronous throws (`NoSuchModelError`); we use Go error returns. Only `LanguageModel` for now -- other model types can be added to the interface when those abstractions exist in the codebase.

The upstream `ProviderV4` has `specificationVersion` and multiple model type methods. We omit `specificationVersion` (not useful in Go where types are checked at compile time) and the other model types (not yet implemented). This is an intentional deviation that keeps the interface minimal until we need more.

### 3. Registry as struct with NewProviderRegistry constructor

`NewProviderRegistry` takes a `map[string]Provider` and variadic options. The struct satisfies the `Provider` interface, enabling composability (registries can be registered inside other registries).

Options via functional options pattern (consistent with the rest of the codebase):
- `WithSeparator(sep string)` -- default `":"`
- `WithLanguageModelMiddleware(mws ...middleware.Middleware)` -- applied to all resolved models

Composite ID resolution: split on first occurrence of separator. Provider ID is everything before, model ID is everything after. This matches upstream `indexOf` behavior and handles model IDs containing the separator character.

### 4. Custom provider as struct with NewCustomProvider constructor

`NewCustomProvider` uses functional options:
- `WithLanguageModels(models map[string]provider.LanguageModel)` -- explicit model map
- `WithFallbackProvider(p Provider)` -- optional fallback for unlisted IDs

Resolution order matches upstream: check explicit map first, then fallback provider, then error. The custom provider also satisfies the `Provider` interface.

### 5. Error handling: sentinel errors with fmt.Errorf wrapping

Following the project's error conventions (sentinel errors + `fmt.Errorf` wrapping, no custom error types):

- `ErrNoSuchModel` -- sentinel for unknown model ID
- `ErrNoSuchProvider` -- sentinel for unknown provider namespace
- `ErrInvalidModelID` -- sentinel for composite IDs missing the separator

Errors are wrapped with context using `fmt.Errorf`: `fmt.Errorf("registry: no such provider %q (available: %v): %w", id, keys, ErrNoSuchProvider)`.

This deviates from upstream's class-based errors (`NoSuchModelError`, `NoSuchProviderError`) but follows Go conventions. Callers use `errors.Is()` to check error types.

### 6. No provider version normalization

The upstream has `asProviderV4()` adapters to normalize V2/V3 providers to V4 at registration time. Our Go port only has V4 (`provider.LanguageModel` is already V4), so no version adapters are needed. If V2/V3 compatibility is ever needed, it would be a separate change.

## Risks / Trade-offs

- **Interface evolution risk**: Starting with `LanguageModel` only means the `Provider` interface will need to grow when embedding/image models are added. This may require a breaking change or a separate `EmbeddingProvider` interface. Mitigation: the interface is in our own package, so we control the evolution. We can add methods with default implementations via wrapper types if needed.

- **Middleware coupling**: The registry imports `middleware` for `WrapLanguageModel`. If the middleware API changes, the registry needs updating. Mitigation: the middleware package is stable and the coupling is minimal (single function call).

- **Separator ambiguity**: Model IDs containing the separator character (e.g., `"provider:model:variant"`) work because we split on the first occurrence only. But this could confuse users. Mitigation: matches upstream behavior exactly, and the error message explains the expected format.
