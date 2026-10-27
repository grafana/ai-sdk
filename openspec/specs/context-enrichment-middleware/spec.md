## Purpose
Context enrichment middleware defines the opt-in nested module for projecting explicitly-approved request context into provider-bound headers and provider options without changing prompts, provider wire defaults, telemetry, or root package APIs.

## Requirements

### Requirement: Nested enrichment middleware module

The repository SHALL provide `middleware/enrichment/` as a separate Go module with module path `github.com/grafana/ai-sdk/middleware/enrichment` and `replace github.com/grafana/ai-sdk => ../../` for local development.

The production package SHALL depend on the ai-sdk root module and the Go standard library only. It SHALL NOT import any provider module. The root ai-sdk module SHALL NOT import `middleware/enrichment`.

#### Scenario: Root consumers do not import enrichment

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk`
- **THEN** `github.com/grafana/ai-sdk/middleware/enrichment` SHALL NOT be pulled into the consumer's transitive dependency graph

#### Scenario: Enrichment module remains provider-agnostic

- **WHEN** the enrichment module's production imports are inspected
- **THEN** no provider module import SHALL be present

### Requirement: Public enrichment middleware API

`middleware/enrichment` SHALL export a provider-agnostic concrete-options API, following the structured logger middleware proposal rather than a generic source/sink pipeline: `Middleware(Options) middleware.Middleware` and `Wrap(provider.LanguageModel, Options) provider.LanguageModel`. The initial API SHALL NOT export `Stack`, `Source`, `SourceFunc`, `Sink`, `Static`, `FromContext`, `HeaderSink`, or `ProviderOptionsSink`.

#### Scenario: Middleware returns TransformParams middleware

- **WHEN** `enrichment.Middleware(opts)` is called
- **THEN** it SHALL return a `middleware.Middleware` whose enrichment behavior is implemented through `TransformParams`
- **AND** it SHALL NOT require `WrapGenerate` or `WrapStream` to modify provider-bound request metadata

#### Scenario: Wrap convenience matches core middleware wrapping

- **WHEN** `enrichment.Wrap(model, opts)` is used
- **THEN** `Wrap` SHALL produce behavior equivalent to wrapping the same model with `enrichment.Middleware(opts)` through the core middleware wrapping utilities

### Requirement: Explicit enrichment value inputs

The enrichment package SHALL provide only explicit value inputs through `Options`. It SHALL support static values through `Options.Values`, context helper values through `Options.ContextValues`, and request-derived values through `Options.DynamicValues`. It SHALL NOT provide a source that implicitly enumerates arbitrary `context.Context` values, all inbound HTTP headers, environment variables, OTel baggage, auth tokens, prompts, tool arguments, or raw user input.

#### Scenario: Static options values are collected

- **WHEN** a middleware is configured with `Options{Values: []Value{{Key: "service", Value: "api"}}}`
- **THEN** that value SHALL be collected for each provider call before filtering is applied

#### Scenario: Context values require explicit opt-in

- **WHEN** a context contains values added by `WithValue` or `WithValues`
- **AND** `Options.ContextValues` is false
- **THEN** those context values SHALL NOT be collected
- **WHEN** `Options.ContextValues` is true
- **THEN** those context values SHALL be collected
- **AND** unrelated context keys SHALL NOT be inspected

#### Scenario: Context values are defensive copies

- **WHEN** a caller mutates a slice returned by `ValuesFromContext`
- **THEN** subsequent calls to `ValuesFromContext` for the same context SHALL NOT observe that mutation

#### Scenario: DynamicValues receives call metadata

- **WHEN** `Options.DynamicValues` runs for a generate or stream call
- **THEN** it SHALL receive `CallInput` containing the call type, current `provider.CallOptions`, and wrapped model

### Requirement: Default-deny filtering and value normalization

The middleware SHALL normalize values before output and derive a filtered slice per output. Eligibility SHALL require `Options.Filter.Include` or that output's mapping; selection by one output alone SHALL NOT authorize another. `Options.Filter.Exclude` SHALL override global include and every output mapping. Filtering SHALL be shallow and string-only; invalid or empty keys SHALL be dropped.

#### Scenario: Values are denied by default

- **WHEN** configured value inputs return values and neither `Options.Filter.Include` nor an output-specific mapping selects their keys
- **THEN** no output SHALL receive those values

#### Scenario: Output-specific mappings do not leak across outputs

- **WHEN** a value key is selected only by `Options.Headers.Map`
- **AND** `Options.ProviderOptions` is also configured
- **THEN** the header output SHALL be eligible to receive that value
- **AND** the provider options output SHALL NOT receive or emit that value unless it is also selected by `Options.ProviderOptions.Map` or included by `Options.Filter.Include`

#### Scenario: Exclude wins over include

- **WHEN** a value key appears in both `Options.Filter.Include` and `Options.Filter.Exclude`
- **THEN** the value SHALL be dropped before output application

#### Scenario: Sensitive values are not emitted raw by default

- **WHEN** an included value has `Sensitive: true` and `Options.Filter.RedactSensitive` is false
- **THEN** the value SHALL be dropped before output application

#### Scenario: Redactor can drop a value

- **WHEN** the configured redactor's `RedactValue` returns `false` for a collected value
- **THEN** that value SHALL be dropped before output application

#### Scenario: High-cardinality dropping is explicit

- **WHEN** an included value has `CardinalityHigh` and `Options.Filter.DropHighCardinality` is true
- **THEN** the value SHALL be dropped before output application

#### Scenario: Over-limit values are dropped

- **WHEN** an included value's post-redaction string length exceeds the configured or default maximum value length
- **THEN** the value SHALL be dropped before output application
- **AND** no output SHALL emit that value

### Requirement: TransformParams enrichment behavior

`Middleware(Options)` SHALL enrich generate and stream `provider.CallOptions` before the inner model. It SHALL copy `Headers` and `ProviderOptions` maps before mutation and SHALL NOT mutate `Prompt`, messages, tools, tool arguments, provider metadata, response metadata, stream parts, or UI/SSE chunks.

#### Scenario: Generate params are enriched

- **WHEN** `DoGenerate` is called on a model wrapped with enrichment middleware
- **THEN** the inner model SHALL receive `provider.CallOptions` with configured header and provider-option enrichment applied

#### Scenario: Stream params are enriched

- **WHEN** `DoStream` is called on a model wrapped with enrichment middleware
- **THEN** the inner model SHALL receive `provider.CallOptions` with configured header and provider-option enrichment applied

#### Scenario: Original maps are not mutated in place

- **WHEN** a call option has existing `Headers` or `ProviderOptions` maps and enrichment applies new values
- **THEN** the returned call options SHALL use copied maps
- **AND** the original maps passed into the middleware SHALL remain unchanged

#### Scenario: Prompt and tools are not mutated

- **WHEN** enrichment applies to a provider call
- **THEN** the prompt, messages, tools, and tool arguments in `provider.CallOptions` SHALL remain semantically unchanged

#### Scenario: Dynamic value error fails closed by default

- **WHEN** `Options.DynamicValues` returns an error and `Options.OnError` is nil
- **THEN** the wrapped model SHALL return an error without invoking the inner model

### Requirement: Header output merge semantics

`Options.Headers` SHALL support enrichment-key-to-header `Map`, optional `Prefix`, `Conflict`, and additional protected names; empty Map and Prefix SHALL disable output. Merge SHALL start from caller headers and detect conflicts case-insensitively via canonical HTTP names. Default `ConflictCallerWins` SHALL preserve caller headers; `ConflictEnrichmentWins` SHALL overwrite only unprotected headers; `ConflictError` SHALL return an error.

#### Scenario: Caller wins by default

- **WHEN** existing call options contain `X-Request-Id: caller` and enrichment would write `X-Request-Id: enriched`
- **THEN** the resulting headers SHALL keep the caller value by default

#### Scenario: Case-insensitive conflicts are detected

- **WHEN** existing call options contain `x-request-id` and enrichment targets `X-Request-Id`
- **THEN** the header output SHALL treat the two names as a conflict

#### Scenario: Enrichment wins when configured

- **WHEN** a header conflict occurs and `Options.Headers` is configured with `ConflictEnrichmentWins`
- **THEN** the resulting headers SHALL use the enrichment value unless the header name is protected

#### Scenario: ConflictError fails the header output

- **WHEN** a header conflict occurs and `Options.Headers` is configured with `ConflictError`
- **THEN** the header output SHALL return an error

#### Scenario: Protected headers are not written when absent

- **WHEN** enrichment targets an absent protected header such as `Authorization` or `X-Access-Token`
- **THEN** the resulting headers SHALL NOT contain that header from enrichment

#### Scenario: Protected headers are not overwritten

- **WHEN** enrichment targets a protected header such as `Authorization` or `X-Access-Token`
- **THEN** the header output SHALL NOT overwrite the existing protected header even if `ConflictEnrichmentWins` is configured

### Requirement: Provider options output merge semantics

`Options.ProviderOptions` SHALL set `ProviderKey`, `ObjectKey`, `Map`, and `Conflict`; empty `ProviderKey` SHALL disable output. String values SHALL be written into `provider.CallOptions.ProviderOptions` under `ProviderKey`: mapped keys use mapped JSON names, globally included unmapped keys use their original keys, and header-only selections SHALL NOT be emitted. Non-empty `ObjectKey` SHALL select a nested object named by `ObjectKey`; empty SHALL select the top-level provider option object.

#### Scenario: Absent provider key creates raw option

- **WHEN** provider options are nil or do not contain the configured provider key
- **THEN** the provider-options output SHALL allocate provider options as needed
- **AND** it SHALL store a `provider.RawProviderOption` containing the enrichment JSON under the configured provider key

#### Scenario: Existing raw object is merged

- **WHEN** provider options contain a `provider.RawProviderOption` with object JSON for the configured provider key
- **THEN** enrichment fields SHALL be shallow-merged into that JSON object or nested object
- **AND** unrelated existing fields SHALL be preserved

#### Scenario: Existing typed option is merged through JSON

- **WHEN** provider options contain a typed provider option for the configured provider key
- **THEN** the provider-options output SHALL marshal the typed option to JSON, merge enrichment into the object JSON, and store the result as `provider.RawProviderOption`

#### Scenario: Existing non-object obeys conflict policy

- **WHEN** provider options contain non-object JSON for the configured provider key
- **THEN** `ConflictCallerWins` SHALL preserve the existing option without enrichment
- **AND** `ConflictEnrichmentWins` SHALL replace it with the enrichment object
- **AND** `ConflictError` SHALL return an error

#### Scenario: ResolveOption remains usable after merge

- **WHEN** a provider option is merged and stored as `provider.RawProviderOption`
- **THEN** downstream code SHALL be able to recover typed views using `provider.ResolveOption` for compatible option structs

### Requirement: Validation and documentation for safe use

The implementation SHALL test safe value collection, filtering, transformation, output merging, and composition. Package godoc SHALL describe enrichment as opt-in, default-deny, string-only, and provider-agnostic, emitting no telemetry and changing no provider/UI representation behavior unless explicitly attached to a model.

#### Scenario: Unit tests cover generate and stream calls

- **WHEN** the enrichment module test suite runs
- **THEN** it SHALL verify that both `DoGenerate` and `DoStream` receive enriched call options when configured

#### Scenario: Documentation warns about sensitive data

- **WHEN** a consumer reads the package documentation
- **THEN** it SHALL explain the default-deny model and warn not to propagate secrets, tokens, prompts, tool arguments, or raw user input

#### Scenario: No default conformance fixture changes are required

- **WHEN** this module remains opt-in without wrapping any model by default
- **THEN** existing provider and UI/SSE conformance fixtures SHALL remain unchanged

#### Scenario: Unit tests exercise filtering and output boundaries
- **WHEN** the enrichment module test suite runs
- **THEN** it SHALL cover value collection, context defensive copies, default-deny filtering, per-output selection isolation, sensitive redaction/drop behavior, cardinality filtering, and over-limit value dropping
- **AND** it SHALL cover generate and stream transformation, header conflict policies, protected headers including absent targets, provider-options creation and merge, unrelated-field preservation, registry composition, and middleware ordering examples

#### Scenario: Godoc states propagation and telemetry boundaries
- **WHEN** a consumer reads package godoc
- **THEN** it SHALL warn against propagating secrets, API tokens, auth claims without explicit filtering, prompts, tool arguments, raw user input, and high-cardinality metric labels
- **AND** it SHALL state that the module emits no telemetry and changes no provider/UI representation behavior unless explicitly attached to a model

### Requirement: Composition with registry and Agent Observability

Enrichment SHALL require no registry changes and SHALL work with `registry.WithLanguageModelMiddleware`, which already accepts `middleware.Middleware`. Docs SHALL explain ordering: before `agentobservability.Stack(...)`, hooks and recording observe enriched `CallOptions`; after Agent Observability, enrichment is transport-only from Agent Observability's perspective.

#### Scenario: Registry applies enrichment to resolved model

- **WHEN** a provider registry is configured with `registry.WithLanguageModelMiddleware(enrichment.Middleware(opts))`
- **THEN** every model resolved by that registry SHALL receive enrichment behavior

#### Scenario: Enrichment before Agent Observability is visible to Agent Observability

- **WHEN** a model is wrapped with enrichment middleware before `agentobservability.Stack(...)`
- **THEN** subsequent Agent Observability middleware SHALL observe the enriched call options

#### Scenario: Enrichment after Agent Observability is transport-only for Agent Observability

- **WHEN** a model is wrapped with Agent Observability middleware before enrichment middleware
- **THEN** Agent Observability middleware SHALL observe the original call options and the inner provider SHALL observe enriched call options

### Requirement: Enrichment value and call types

The enrichment API SHALL expose typed value metadata and request-aware dynamic collection.

#### Scenario: Configure typed enrichment values
- **WHEN** a caller constructs enrichment values and metadata options
- **THEN** the package SHALL expose `Cardinality` as a named string enum with typed constants `CardinalityLow`, `CardinalityBounded`, and `CardinalityHigh`
- **AND** `Value` SHALL have fields `Key string`, `Value string`, `Sensitive bool`, and `Cardinality Cardinality`
- **AND** it SHALL expose `ValueOption`, `Sensitive() ValueOption`, and `WithCardinality(Cardinality) ValueOption` for `WithValue` metadata

#### Scenario: Configure dynamic request collection
- **WHEN** a caller supplies a dynamic collector
- **THEN** the API SHALL expose `DynamicValuesFunc func(context.Context, CallInput) ([]Value, error)`
- **AND** `CallInput` SHALL have fields `Type middleware.CallType`, `Params provider.CallOptions`, and `Model provider.LanguageModel`

### Requirement: Enrichment configuration types

The enrichment API SHALL expose concrete options for selection, redaction, outputs, and error handling.

#### Scenario: Configure filtering and outputs
- **WHEN** a caller constructs enrichment output and filter configuration
- **THEN** `FilterOptions` SHALL have fields for include/exclude filtering, sensitive redaction, high-cardinality dropping, and value length limits
- **AND** `HeaderOptions` SHALL have fields `Map map[string]string`, `Prefix string`, `Conflict ConflictPolicy`, and `AdditionalProtected []string`
- **AND** `ProviderOptionsConfig` SHALL have fields `ProviderKey string`, `ObjectKey string`, `Map map[string]string`, and `Conflict ConflictPolicy`
- **AND** `ConflictPolicy` SHALL be a named string enum with typed constants `ConflictCallerWins`, `ConflictEnrichmentWins`, and `ConflictError`

#### Scenario: Configure middleware options and redactor
- **WHEN** a caller constructs `Options` for enrichment middleware
- **THEN** `Options` SHALL have fields `Values []Value`, `ContextValues bool`, `DynamicValues DynamicValuesFunc`, `Headers HeaderOptions`, `ProviderOptions ProviderOptionsConfig`, `Filter FilterOptions`, `Redactor Redactor`, and `OnError func(context.Context, error) error`
- **AND** the package SHALL expose a `Redactor` interface with `RedactValue(context.Context, Value) (Value, bool)`, a `RedactorFunc` adapter, and `DefaultRedactor() Redactor`

### Requirement: Enrichment context helper API

Context helpers SHALL use unexported context key types and defensive copies.

#### Scenario: Store and retrieve explicit context values
- **WHEN** a caller stores enrichment values in a context and retrieves them
- **THEN** the package SHALL expose `WithValue(ctx context.Context, key, value string, opts ...ValueOption) context.Context`, `WithValues(ctx context.Context, values ...Value) context.Context`, and `ValuesFromContext(ctx context.Context) []Value`
- **AND** storage and retrieval SHALL use defensive copies and unexported context key types

### Requirement: Enrichment sensitive value redaction

Sensitive values SHALL NOT be emitted raw by default. `RedactSensitive=false` SHALL drop sensitive values; true SHALL allow them only after redaction. A nil `Options.Redactor` SHALL use `DefaultRedactor()`. Redactors SHALL be able to transform, mark sensitive, or drop values by returning false. The default redactor SHALL mark known secret-looking keys sensitive and otherwise leave values unchanged.

#### Scenario: Redact an included sensitive value
- **WHEN** an included value is sensitive and `Options.Filter.RedactSensitive` is true
- **THEN** it SHALL be eligible for emission only after redaction

#### Scenario: Default redactor detects secret keys
- **WHEN** `Options.Redactor` is nil and collection includes secret-looking and ordinary keys
- **THEN** `DefaultRedactor()` SHALL mark known secret-looking keys sensitive and otherwise leave values unchanged

### Requirement: Enrichment value length and cardinality limits

Values SHALL have a documented non-zero default maximum length unless `Options.Filter.MaxValueLength` overrides it. Length enforcement SHALL run after redaction; over-limit values SHALL be dropped without emission. High-cardinality values SHALL be allowed for header/provider-option outputs by default; `DropHighCardinality=true` SHALL drop `CardinalityHigh` values.

#### Scenario: Redaction precedes length enforcement
- **WHEN** redaction changes an included value's length
- **THEN** the configured or documented non-zero default maximum length SHALL be checked after redaction
- **AND** an over-limit value SHALL be dropped without output emission

#### Scenario: High cardinality is allowed by default
- **WHEN** an eligible value has `CardinalityHigh` and `DropHighCardinality` is false
- **THEN** header/provider-option outputs SHALL allow that value

### Requirement: Enrichment error handling

Dynamic collection or header/provider-options output errors SHALL fail the model call by default. When set, `Options.OnError` SHALL receive the context and error; a non-nil returned error SHALL fail the call with that error, and nil SHALL proceed with the last successfully built call options.

#### Scenario: Error handler permits a call
- **WHEN** dynamic collection or output application returns an error and `Options.OnError` returns nil
- **THEN** the call SHALL proceed with the last successfully built call options

#### Scenario: Error handler replaces a failure
- **WHEN** dynamic collection or output application returns an error and `Options.OnError` returns a non-nil error
- **THEN** the call SHALL fail with that returned error

### Requirement: Protected enrichment headers

Protected auth/transport headers SHALL NOT be written or overwritten by default regardless of conflict policy, even when absent. Callers SHALL be able to add protected names, but the initial API SHALL NOT allow writing built-in protected names.

#### Scenario: Built-in and deployment-specific headers are protected
- **WHEN** enrichment targets auth or transport headers
- **THEN** the protected set SHALL include common auth and provider transport headers such as `Authorization`, `Proxy-Authorization`, `X-Access-Token`, `X-Grafana-Id`, `Content-Type`, and provider API-key or protocol headers
- **AND** caller-added deployment-specific protected header names SHALL also be protected
- **AND** enrichment SHALL omit absent protected targets and SHALL NOT overwrite existing protected values regardless of conflict policy

### Requirement: Existing provider option preservation

Provider-option output SHALL preserve unrelated fields. Existing typed or raw options SHALL be marshaled to JSON and require object-shaped JSON for merging; merged data SHALL be stored as `provider.RawProviderOption{Key: ProviderKey, Raw: mergedJSON}`. Field conflicts SHALL follow `ConflictPolicy`, defaulting to `ConflictCallerWins`.

#### Scenario: Merge a conflicting provider option field
- **WHEN** enrichment and an existing object option supply the same field
- **THEN** `ConflictCallerWins` SHALL preserve the caller field by default
- **AND** `ConflictEnrichmentWins` SHALL replace it with the enrichment value
- **AND** `ConflictError` SHALL return an error
- **AND** unrelated existing fields SHALL remain unchanged
