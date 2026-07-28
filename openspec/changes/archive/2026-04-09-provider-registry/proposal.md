## Why

The Go port currently has no provider management. Callers instantiate provider models directly (e.g., `anthropic.New(key, modelID)`), which works for single-provider setups but becomes unwieldy when applications use multiple providers, need aliased/pre-configured models, or want configuration-driven model selection. The upstream Vercel AI SDK provides centralized provider management (registry + custom provider) that we need to port for feature parity and wire compatibility.

## What Changes

- Introduce a `Provider` interface for string-based model resolution (`LanguageModel(id string) (provider.LanguageModel, error)`)
- Implement a provider registry that maps provider namespaces to `Provider` implementations, resolving composite IDs like `"anthropic:claude-sonnet-4-6"`
- Implement a custom provider for model aliasing, pre-configured defaults, and access control
- The registry supports configurable separators (default `":"`), optional middleware wrapping, and composability (registry satisfies `Provider`)
- Add sentinel errors for unknown provider and unknown model resolution failures

## Capabilities

### New Capabilities
- `provider-registry`: Provider registry for multi-provider routing via composite string IDs, with configurable separator and optional middleware wrapping
- `custom-provider`: Custom provider for model aliasing, fallback delegation, and access control through explicit model registration

### Modified Capabilities

## Impact

- New `registry` package in the root module with `Provider` interface, `ProviderRegistry`, and `CustomProvider`
- Depends on `provider` package (for `LanguageModel` interface) and `middleware` package (for optional model wrapping)
- No breaking changes to existing APIs -- `StreamText` and `GenerateText` continue to accept `provider.LanguageModel` directly
- The anthropic module would benefit from implementing the `Provider` interface, but that can be done separately
