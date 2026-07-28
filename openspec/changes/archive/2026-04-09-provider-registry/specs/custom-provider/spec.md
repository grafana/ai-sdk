## ADDED Requirements

### Requirement: Custom provider creation
The `registry` package SHALL export a `NewCustomProvider` function that accepts variadic options and returns a `Provider` implementation for model aliasing, pre-configured defaults, and access control.

#### Scenario: Create custom provider with language models
- **WHEN** `NewCustomProvider(WithLanguageModels(map[string]provider.LanguageModel{"fast": haiku, "reasoning": opus}))` is called
- **THEN** it SHALL return a `Provider` that resolves `"fast"` and `"reasoning"` to the corresponding models

### Requirement: Explicit model resolution
The custom provider SHALL resolve model IDs by looking up the explicit model map first.

#### Scenario: Model found in explicit map
- **WHEN** `provider.LanguageModel("fast")` is called and `"fast"` is in the language models map
- **THEN** it SHALL return the mapped `provider.LanguageModel` and nil error

### Requirement: Fallback provider delegation
The custom provider SHALL support an optional fallback provider via `WithFallbackProvider(p Provider)` that is consulted when a model ID is not found in the explicit map.

#### Scenario: Model not in map but fallback resolves it
- **WHEN** `provider.LanguageModel("claude-sonnet-4-6")` is called, `"claude-sonnet-4-6"` is not in the explicit map, and a fallback provider is configured
- **THEN** it SHALL delegate to the fallback provider's `LanguageModel("claude-sonnet-4-6")` and return its result

#### Scenario: Model not in map and fallback fails
- **WHEN** `provider.LanguageModel("unknown")` is called, `"unknown"` is not in the explicit map, and the fallback provider returns an error
- **THEN** it SHALL return the error from the fallback provider

### Requirement: Access control without fallback
When no fallback provider is configured, the custom provider SHALL only resolve model IDs that are explicitly registered, rejecting all others.

#### Scenario: No fallback and model not in map
- **WHEN** `provider.LanguageModel("unrestricted-model")` is called, `"unrestricted-model"` is not in the explicit map, and no fallback is configured
- **THEN** it SHALL return a nil model and an error wrapping `ErrNoSuchModel`

#### Scenario: Access control restricts to allowed models
- **WHEN** a custom provider is created with only `"claude-sonnet-4-6"` in the map and no fallback
- **THEN** only `"claude-sonnet-4-6"` SHALL be resolvable; all other model IDs SHALL return an error

### Requirement: Custom provider satisfies Provider interface
The value returned by `NewCustomProvider` SHALL satisfy the `Provider` interface, allowing it to be registered in a `ProviderRegistry`.

#### Scenario: Custom provider in registry
- **WHEN** a custom provider is registered in a `ProviderRegistry` under namespace `"my"`
- **THEN** `registry.LanguageModel("my:fast")` SHALL resolve through the custom provider's model map
