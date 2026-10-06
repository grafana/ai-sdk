# Gateway Model Catalog

## Purpose

Define a transport-neutral public model catalog for hosted gateways, including canonical resolution, stable discovery metadata, aliases, structured unknown-model errors, registry-provider adaptation, and host-owned transport composition.

## Requirements

### Requirement: Request-aware gateway model contracts
The `gateway/catalog` package SHALL expose a `ModelResolver` contract that resolves a public model ID with `context.Context`, a `ModelLister` contract that lists visible model metadata with `context.Context`, and a `Catalog` contract that combines both capabilities.

#### Scenario: Resolver receives request context
- **WHEN** a consumer resolves a public model ID through `ModelResolver`
- **THEN** the consumer SHALL supply a `context.Context` that a request-policy decorator can inspect

#### Scenario: Lister receives request context
- **WHEN** a consumer lists models through `ModelLister`
- **THEN** the consumer SHALL supply a `context.Context` so listing can apply the same request-scoped visibility policy as resolution

#### Scenario: Consumer depends only on resolution
- **WHEN** a consumer requires model lookup but not discovery
- **THEN** it SHALL be able to depend on `ModelResolver` without implementing or accepting the listing contract

### Requirement: Canonical resolved model identity
A successful resolution SHALL return both the canonical public catalog ID and a non-nil `provider.LanguageModel`. The canonical public ID SHALL be independent of the model-reported, provider-specific value returned by `LanguageModel.ModelID()`.

#### Scenario: Public and model-reported IDs differ
- **WHEN** a public ID resolves to a model whose reported model ID differs
- **THEN** the result SHALL contain the public canonical ID and the unchanged provider model

#### Scenario: Alias resolves to canonical identity
- **WHEN** a registered alias is resolved
- **THEN** the result SHALL contain the canonical entry ID rather than the alias

### Requirement: Immutable static catalog
The package SHALL provide a static catalog constructor that copies its entries and nested metadata slices, stores only non-nil models, and exposes no mutation API.

#### Scenario: Source entries are mutated after construction
- **WHEN** the caller mutates the input entries, aliases, or capabilities after successful construction
- **THEN** subsequent catalog resolution and listing SHALL remain unchanged

#### Scenario: Static model resolves
- **WHEN** a canonical ID or registered alias identifies a static entry
- **THEN** the catalog SHALL return that entry's canonical ID and model

#### Scenario: Request context is not retained
- **WHEN** the static catalog resolves or lists with a request context
- **THEN** it SHALL complete without storing the context in catalog state

### Requirement: Catalog namespace validation
Catalog constructors SHALL reject empty canonical IDs, empty aliases, duplicate canonical IDs, duplicate aliases, aliases that collide with canonical IDs, and entries with unavailable resolution dependencies.

#### Scenario: Canonical IDs collide
- **WHEN** two entries declare the same canonical ID
- **THEN** construction SHALL fail with an error identifying the conflicting ID

#### Scenario: Alias collides with an ID
- **WHEN** an alias equals any canonical ID or another entry's alias
- **THEN** construction SHALL fail with an error identifying the collision

#### Scenario: Static model is nil
- **WHEN** a static entry contains a nil `provider.LanguageModel`
- **THEN** construction SHALL fail instead of silently omitting the entry

#### Scenario: Registry provider is nil
- **WHEN** a registry-backed catalog is constructed without a `registry.Provider`
- **THEN** construction SHALL fail

#### Scenario: Registry route target is empty
- **WHEN** a registry route has an empty provider model ID
- **THEN** construction SHALL fail with an error identifying the route's canonical public ID

### Requirement: Stable model listing
Listing SHALL return one `ModelInfo` per canonical entry in ascending canonical-ID order. Each entry SHALL include its canonical ID and SHALL preserve optional name, description, aliases, and capabilities supplied at construction.

#### Scenario: Catalog contains aliases
- **WHEN** models are listed
- **THEN** aliases SHALL appear as metadata on their canonical entry and SHALL NOT appear as separate model rows

#### Scenario: Listing result is mutated
- **WHEN** a caller mutates the returned list, aliases, or capabilities
- **THEN** later listing and resolution SHALL remain unchanged

#### Scenario: Empty catalog is listed
- **WHEN** a valid catalog has no entries
- **THEN** listing SHALL return an empty list without an error

### Requirement: Public model metadata semantics
`ModelInfo` SHALL require a canonical public ID and SHALL support optional presentation name, description, explicit aliases, and typed model capabilities. Metadata SHALL describe the public route rather than exposing or inferring provider-specific invocation or routing details. Canonical IDs, aliases, and capabilities SHALL be supplied by the catalog owner; the catalog SHALL NOT derive them from `LanguageModel.ModelID()`, provider `ModelIDs()` inventories, or built-in public-name policy.

#### Scenario: Provider inventories do not create public routes
- **WHEN** provider packages expose supported model ID inventories
- **THEN** the catalog SHALL NOT register those IDs or infer public names unless the catalog owner supplies explicit entries

#### Scenario: Fallback route declares capabilities
- **WHEN** a public route can select more than one backend model
- **THEN** its declared capabilities SHALL represent behavior guaranteed by every possible backend

#### Scenario: Presentation metadata is omitted
- **WHEN** an entry supplies only its required canonical ID
- **THEN** the catalog SHALL accept and list the entry without inventing provider-derived metadata

#### Scenario: Model-reported identity differs
- **WHEN** the resolved model reports a provider-specific identity
- **THEN** listing metadata SHALL remain the explicitly configured public metadata

### Requirement: Structured unknown-model errors
The package SHALL expose an `ErrUnknownModel` sentinel and return a pointer `*UnknownModelError` that contains the requested public ID and unwraps to the sentinel. Unknown-model errors SHALL NOT enumerate available catalog entries.

#### Scenario: Unknown canonical ID
- **WHEN** resolution receives an ID that is neither canonical nor an alias
- **THEN** it SHALL return a nil model result and an error that matches `ErrUnknownModel` with `errors.Is`

#### Scenario: Caller inspects requested ID
- **WHEN** a caller declares `var target *catalog.UnknownModelError` and calls `errors.As(err, &target)` on an unknown-model error
- **THEN** it SHALL match the pointer error and expose the exact requested public ID

#### Scenario: Unknown error does not disclose models
- **WHEN** unknown-model error text is returned
- **THEN** it SHALL identify the requested ID without including the catalog's available IDs

### Requirement: Registry provider adapter
The package SHALL provide a registry-backed catalog that accepts any `registry.Provider` and explicit routes from public entries to opaque provider model IDs. It SHALL pass the configured provider model ID unchanged to `registry.Provider.LanguageModel` at resolution time.

#### Scenario: Public ID maps to composite registry ID
- **WHEN** a route maps public `claude-opus-4-8` to `anthropic:claude-opus-4-8`
- **THEN** resolution SHALL call the provider with `anthropic:claude-opus-4-8` and return `claude-opus-4-8` as the canonical public ID

#### Scenario: Public alias maps through registry
- **WHEN** a route alias is resolved
- **THEN** the adapter SHALL resolve the route's configured provider model ID and return the canonical public ID

#### Scenario: Custom provider is adapted
- **WHEN** the adapter receives a `registry.Provider` implementation other than `*registry.ProviderRegistry`
- **THEN** it SHALL resolve routes without requiring concrete registry internals

### Requirement: Registry errors remain distinct from unknown public routes
The registry adapter SHALL return `UnknownModelError` only when the requested public ID or alias is absent. Errors returned by `registry.Provider.LanguageModel` SHALL be preserved with gateway route context and SHALL remain identifiable through their original sentinel or cause.

#### Scenario: Registry rejects configured route
- **WHEN** a public route exists but its configured provider model ID cannot be resolved
- **THEN** the adapter SHALL return an error preserving the registry failure and SHALL NOT replace it with `ErrUnknownModel`

#### Scenario: Provider returns nil without error
- **WHEN** a public route exists but the provider returns a nil model and nil error
- **THEN** the adapter SHALL return an invalid-provider-result error and SHALL NOT classify the public route as unknown

### Requirement: Catalog remains separate from registry and transport policy
The gateway catalog SHALL treat public IDs as opaque strings and SHALL NOT require registry separator syntax. This capability SHALL NOT change `registry.Provider`, `ProviderRegistry`, `CustomProvider`, provider-wire routes or payloads, or the behavior of `providers/grafana`.

#### Scenario: Public ID contains no provider separator
- **WHEN** a static or registry-backed catalog registers a flat public ID
- **THEN** it SHALL resolve that ID without requiring `provider:model` syntax

#### Scenario: Existing registry is used directly
- **WHEN** an existing consumer calls `ProviderRegistry.LanguageModel` without using a gateway catalog
- **THEN** its composite-ID resolution and middleware behavior SHALL remain unchanged

#### Scenario: Catalog resolves a model
- **WHEN** a catalog returns an existing `provider.LanguageModel`
- **THEN** it SHALL NOT alter provider call options, provider requests, stream parts, UI chunks, SSE framing, or provider-wire serialization

### Requirement: Provider-wire server composition remains host-owned
The `gateway/catalog` package SHALL remain independent of `net/http` and `gateway/providerwire`. When a host composes `catalog.ModelResolver` with the separately proposed `providerwire.ModelResolver`, a host-owned adapter SHALL pass `r.Context()` to catalog resolution, return the resolved language model for execution, and preserve canonical public identity for host-owned policy or logging. The adapter SHALL translate only `catalog.ErrUnknownModel` into a non-retryable HTTP 404 `*provider.APICallError` with the catalog error as its cause; other catalog or registry failures SHALL pass through unchanged.

#### Scenario: Catalog dependency boundary
- **WHEN** imports and public types in the `gateway/catalog` package are inspected
- **THEN** they SHALL NOT import or expose `net/http` or `gateway/providerwire`

#### Scenario: Successful provider-wire adaptation
- **WHEN** a valid provider-wire request is adapted to a gateway catalog resolver
- **THEN** the adapter SHALL resolve with the original request context, return `ResolvedModel.Model` to the HTTP execution boundary, and make `ResolvedModel.ID` available to host-owned policy or logging

#### Scenario: Unknown public model maps to HTTP 404
- **WHEN** catalog resolution returns an error matching `catalog.ErrUnknownModel`
- **THEN** the host adapter SHALL return a non-retryable HTTP 404 `*provider.APICallError` whose cause is the original catalog error

#### Scenario: Non-catalog failure retains provider-wire normalization
- **WHEN** a configured public route returns a registry or provider failure that does not match `catalog.ErrUnknownModel`
- **THEN** the host adapter SHALL pass that error through unchanged rather than misclassifying it as an unknown public model

### Requirement: Assistant-specific policy remains external
The gateway catalog SHALL NOT define Assistant model families, profile slots, `chat-large`/`chat-small` selection, Claude alias tables, entitlement rules, provider credentials, provider ordering, or fallback construction.

#### Scenario: Assistant supplies aliases
- **WHEN** the Assistant service requires Claude aliases
- **THEN** it SHALL configure those aliases as consumer-owned catalog data rather than relying on built-in ai-sdk policy

#### Scenario: Assistant applies entitlements
- **WHEN** the Assistant service restricts models per request
- **THEN** it SHALL implement that policy outside ai-sdk by decorating resolution and listing consistently
