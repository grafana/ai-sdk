## 1. Package Setup and Core Types

- [x] 1.1 Create `registry/` package with `doc.go` and sentinel errors (`ErrNoSuchModel`, `ErrNoSuchProvider`, `ErrInvalidModelID`)
- [x] 1.2 Define the `Provider` interface with `LanguageModel(modelID string) (provider.LanguageModel, error)`

## 2. Provider Registry

- [x] 2.1 Implement `ProviderRegistry` struct with `NewProviderRegistry(providers map[string]Provider, opts ...RegistryOption)` constructor
- [x] 2.2 Implement composite ID resolution (`splitID`) -- split on first separator occurrence, default `":"`
- [x] 2.3 Implement `LanguageModel` method -- split ID, look up provider, delegate, apply middleware if configured
- [x] 2.4 Implement `WithSeparator` and `WithLanguageModelMiddleware` functional options
- [x] 2.5 Write tests for registry: standard resolution, custom separator, missing separator error, unknown provider error, middleware wrapping, composability (nested registries)

## 3. Custom Provider

- [x] 3.1 Implement `customProvider` struct with `NewCustomProvider(opts ...CustomProviderOption)` constructor
- [x] 3.2 Implement `LanguageModel` method -- check explicit map, then fallback, then error
- [x] 3.3 Implement `WithLanguageModels` and `WithFallbackProvider` functional options
- [x] 3.4 Write tests for custom provider: explicit resolution, fallback delegation, access control (no fallback), unknown model error, integration with registry

## 4. Integration and Verification

- [x] 4.1 Add compile-time interface satisfaction checks (`var _ Provider = (*ProviderRegistry)(nil)`, `var _ Provider = ...`)
- [x] 4.2 Run `make check` to verify build, fmt, vet, and all tests pass
