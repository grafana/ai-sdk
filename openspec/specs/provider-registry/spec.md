## Purpose

Define provider registration and model resolution, including composite identifiers, configurable separators, middleware, nested registries, and sentinel errors.

## Requirements

### Requirement: Provider interface
The `registry` package SHALL export a `Provider` interface with a single method `LanguageModel(modelID string) (provider.LanguageModel, error)` that resolves a language model by string ID.

#### Scenario: Provider resolves known model
- **WHEN** a `Provider` implementation is called with a model ID it recognizes
- **THEN** it SHALL return the corresponding `provider.LanguageModel` and a nil error

#### Scenario: Provider rejects unknown model
- **WHEN** a `Provider` implementation is called with a model ID it does not recognize
- **THEN** it SHALL return a nil model and an error wrapping `ErrNoSuchModel`

### Requirement: Provider registry creation
The `registry` package SHALL export a `NewProviderRegistry` function that accepts a `map[string]Provider` of named providers and variadic options, returning a `*ProviderRegistry` that satisfies the `Provider` interface.

#### Scenario: Create registry with multiple providers
- **WHEN** `NewProviderRegistry` is called with a map containing providers keyed by namespace (e.g., `"anthropic"`, `"openai"`)
- **THEN** it SHALL return a `*ProviderRegistry` that can resolve models from any registered provider

### Requirement: Composite ID resolution
The provider registry SHALL resolve composite model IDs by splitting on the first occurrence of the separator (default `":"`), using the prefix as the provider namespace and the suffix as the model ID passed to that provider.

#### Scenario: Resolve standard composite ID
- **WHEN** `registry.LanguageModel("anthropic:claude-sonnet-4-6")` is called with default separator
- **THEN** it SHALL look up the `"anthropic"` provider and call its `LanguageModel("claude-sonnet-4-6")` method

#### Scenario: Model ID contains separator character
- **WHEN** `registry.LanguageModel("provider:model:variant")` is called
- **THEN** it SHALL split on the first `":"` only, passing `"model:variant"` as the model ID to the `"provider"` provider

#### Scenario: Composite ID missing separator
- **WHEN** `registry.LanguageModel("no-separator")` is called
- **THEN** it SHALL return an error wrapping `ErrInvalidModelID` with a message explaining the expected format `"providerId:modelId"`

### Requirement: Unknown provider error
The provider registry SHALL return a descriptive error when the provider namespace in a composite ID does not match any registered provider.

#### Scenario: Provider namespace not registered
- **WHEN** `registry.LanguageModel("unknown:model")` is called and `"unknown"` is not a registered provider
- **THEN** it SHALL return an error wrapping `ErrNoSuchProvider` that includes the unknown provider ID and the list of available provider names

### Requirement: Configurable separator
The provider registry SHALL support a configurable separator via `WithSeparator(sep string)` option, defaulting to `":"`.

#### Scenario: Custom separator
- **WHEN** a registry is created with `WithSeparator(" > ")` and `LanguageModel("anthropic > claude-sonnet-4-6")` is called
- **THEN** it SHALL split on `" > "` and resolve `"anthropic"` provider with model ID `"claude-sonnet-4-6"`

### Requirement: Registry middleware support
The provider registry SHALL support optional language model middleware via `WithLanguageModelMiddleware(mws ...middleware.Middleware)` option that wraps every resolved model.

#### Scenario: Middleware applied to resolved models
- **WHEN** a registry is created with middleware and `LanguageModel` is called
- **THEN** the returned model SHALL be wrapped with the specified middleware via `middleware.WrapLanguageModel`

#### Scenario: No middleware configured
- **WHEN** a registry is created without middleware and `LanguageModel` is called
- **THEN** the returned model SHALL be the unwrapped model from the provider

### Requirement: Registry composability
The `*ProviderRegistry` type SHALL satisfy the `Provider` interface, allowing registries to be nested inside other registries.

#### Scenario: Nested registry
- **WHEN** a `ProviderRegistry` is registered as a provider inside another `ProviderRegistry`
- **THEN** the outer registry SHALL delegate to the inner registry's `LanguageModel` method for resolution

### Requirement: Sentinel errors
The `registry` package SHALL export sentinel errors: `ErrNoSuchModel`, `ErrNoSuchProvider`, and `ErrInvalidModelID`. All error returns from registry and custom provider SHALL wrap one of these sentinels so callers can use `errors.Is()`.

#### Scenario: Error identification with errors.Is
- **WHEN** a caller receives an error from `LanguageModel` for an unknown provider
- **THEN** `errors.Is(err, registry.ErrNoSuchProvider)` SHALL return true
- **AND** `errors.Is(err, registry.ErrNoSuchModel)` SHALL also return true (since unknown provider implies unknown model)
