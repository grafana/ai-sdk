## ADDED Requirements

### Requirement: Nested enrichment middleware module

The repository SHALL provide `middleware/enrichment/` as a separate Go module with module path `github.com/grafana/ai-sdk/middleware/enrichment` and `replace github.com/grafana/ai-sdk => ../../` for local development.

The production package SHALL depend on the ai-sdk root module and the Go standard library only. It SHALL NOT import `github.com/grafana/ai-sdk/providers/grafana`, `github.com/grafana/ai-sdk/providers/anthropic`, or any other provider module. The root ai-sdk module SHALL NOT import `middleware/enrichment`.

#### Scenario: Root consumers do not import enrichment

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk`
- **THEN** `github.com/grafana/ai-sdk/middleware/enrichment` SHALL NOT be pulled into the consumer's transitive dependency graph

#### Scenario: Enrichment module remains provider-agnostic

- **WHEN** the enrichment module's production imports are inspected
- **THEN** no provider module import SHALL be present

### Requirement: Public enrichment middleware API

The `middleware/enrichment` package SHALL export a provider-agnostic API for collecting enrichment values and applying them to provider call options. The public API SHALL follow the concrete-options style used by the structured logger middleware proposal rather than exposing a generic source/sink pipeline.

Functions SHALL include `Middleware(Options) middleware.Middleware` and `Wrap(provider.LanguageModel, Options) provider.LanguageModel`. The initial API SHALL NOT export `Stack`, `Source`, `SourceFunc`, `Sink`, `Static`, `FromContext`, `HeaderSink`, or `ProviderOptionsSink`.

Types SHALL include:

- `Cardinality` as a named string enum with typed constants `CardinalityLow`, `CardinalityBounded`, and `CardinalityHigh`.
- `Value` with fields `Key string`, `Value string`, `Sensitive bool`, and `Cardinality Cardinality`.
- `ValueOption` plus helpers `Sensitive() ValueOption` and `WithCardinality(Cardinality) ValueOption` for `WithValue` metadata.
- `CallInput` with fields `Type middleware.CallType`, `Params provider.CallOptions`, and `Model provider.LanguageModel`.
- `DynamicValuesFunc func(context.Context, CallInput) ([]Value, error)`.
- `FilterOptions` with fields for include/exclude filtering, sensitive redaction, high-cardinality dropping, and value length limits.
- `HeaderOptions` with fields `Map map[string]string`, `Prefix string`, `Conflict ConflictPolicy`, and `AdditionalProtected []string`.
- `ProviderOptionsConfig` with fields `ProviderKey string`, `ObjectKey string`, `Map map[string]string`, and `Conflict ConflictPolicy`.
- `Redactor` interface with `RedactValue(context.Context, Value) (Value, bool)`.
- `RedactorFunc` adapter type.
- `DefaultRedactor() Redactor`.
- `ConflictPolicy` as a named string enum with typed constants `ConflictCallerWins`, `ConflictEnrichmentWins`, and `ConflictError`.
- `Options` with fields `Values []Value`, `ContextValues bool`, `DynamicValues DynamicValuesFunc`, `Headers HeaderOptions`, `ProviderOptions ProviderOptionsConfig`, `Filter FilterOptions`, `Redactor Redactor`, and `OnError func(context.Context, error) error`.

Context helpers SHALL use unexported context key types and defensive copies. The package SHALL export `WithValue(ctx context.Context, key, value string, opts ...ValueOption) context.Context`, `WithValues(ctx context.Context, values ...Value) context.Context`, and `ValuesFromContext(ctx context.Context) []Value`.

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

The enrichment middleware SHALL normalize collected values before any output is applied, then derive a per-output filtered value slice. A value SHALL be eligible for an output only when its key is explicitly included by `Options.Filter.Include` or selected by that same output's mapping. A value selected only by one output mapping SHALL NOT be emitted to any other output. `Options.Filter.Exclude` SHALL take precedence over both global include and output-specific mappings for every output.

Filtering SHALL be shallow and string-only. Invalid or empty keys SHALL be dropped. Values SHALL be subject to a documented non-zero default maximum length unless `Options.Filter.MaxValueLength` overrides it. Length enforcement SHALL run after redaction; over-limit values SHALL be dropped without output emission.

Sensitive values SHALL NOT be emitted in raw form by default. If `Options.Filter.RedactSensitive` is false, sensitive values SHALL be dropped. If `Options.Filter.RedactSensitive` is true, sensitive values SHALL be emitted only after redaction. If `Options.Redactor` is nil, the middleware SHALL use `DefaultRedactor()`. A redactor SHALL be able to transform a value, mark it sensitive, or drop it by returning `false`. `DefaultRedactor()` SHALL mark known secret-looking key names as sensitive and otherwise leave values unchanged.

High-cardinality values SHALL be allowed for header/provider-option outputs by default. If `Options.Filter.DropHighCardinality` is true, values with `CardinalityHigh` SHALL be dropped.

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

`Middleware(Options)` SHALL enrich both generate and stream calls by transforming `provider.CallOptions` before they reach the inner model. The transformation SHALL copy `Headers` and `ProviderOptions` maps before mutating them. It SHALL NOT mutate `Prompt`, messages, tools, tool arguments, provider metadata, response metadata, stream parts, or UI/SSE chunks.

If dynamic value collection or header/provider-options output application returns an error, the middleware SHALL fail the model call by default. If `Options.OnError` is set, the middleware SHALL call it with the context and error; a non-nil returned error SHALL fail the call with that error, and a nil returned error SHALL allow the call to proceed with the last successfully built call options.

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

The package SHALL provide header enrichment through `Options.Headers`. `HeaderOptions` SHALL support explicit `Map map[string]string` mode from enrichment key to HTTP header name, optional `Prefix string` mode, `Conflict ConflictPolicy`, and additional protected header names. Header output SHALL be disabled when both `Map` and `Prefix` are empty.

Header merge SHALL start from existing caller headers. Header conflict detection SHALL be case-insensitive using canonical HTTP header names. The default conflict policy SHALL be `ConflictCallerWins`.

For conflicts:

- `ConflictCallerWins` SHALL preserve the existing caller header.
- `ConflictEnrichmentWins` SHALL overwrite the existing caller header unless the target header is protected.
- `ConflictError` SHALL return an error.

Protected auth and transport headers SHALL NOT be written or overwritten by enrichment by default, regardless of conflict policy. If enrichment targets a protected header and no caller value exists, the header output SHALL still omit that enrichment header. The protected set SHALL include common auth and provider transport headers such as `Authorization`, `Proxy-Authorization`, `X-Access-Token`, `X-Grafana-Id`, `Content-Type`, provider API-key headers, and Grafana provider wire headers. Callers SHALL be able to add deployment-specific protected header names, but the initial API SHALL NOT provide an opt-in to write built-in protected header names.

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

The package SHALL provide provider-options enrichment through `Options.ProviderOptions`. `ProviderOptionsConfig` SHALL include `ProviderKey string`, `ObjectKey string`, `Map map[string]string`, and `Conflict ConflictPolicy`. Provider-options output SHALL be disabled when `ProviderKey` is empty.

The output SHALL write string enrichment values into `provider.CallOptions.ProviderOptions` under `ProviderKey`. Values selected by `ProviderOptionsConfig.Map` SHALL write to the corresponding JSON field names. Globally included values not present in the map SHALL write under their original keys. Values selected solely by the header mapping SHALL NOT be emitted into provider options. If `ObjectKey` is non-empty, the output SHALL write values into a nested object named by `ObjectKey`; otherwise it SHALL write values into the top-level provider option object.

The output SHALL preserve unrelated existing fields. When the provider key exists, it SHALL marshal the existing typed or raw provider option to JSON, require object-shaped JSON for merging, and store the merged result as `provider.RawProviderOption{Key: ProviderKey, Raw: mergedJSON}`. Field conflicts SHALL obey `ConflictPolicy`, with `ConflictCallerWins` as the default.

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

#### Scenario: Grafana controls are preserved

- **WHEN** provider options contain existing `grafana.sigil`, `grafana.tracing`, `grafana.metrics`, or `grafana.usage` fields
- **AND** enrichment writes to `ProviderKey: "grafana"` and `ObjectKey: "enrichment"`
- **THEN** those existing Grafana hosted middleware control fields SHALL be preserved unchanged

#### Scenario: ResolveOption remains usable after merge

- **WHEN** a provider option is merged and stored as `provider.RawProviderOption`
- **THEN** downstream code SHALL be able to recover typed views using `provider.ResolveOption` for compatible option structs

### Requirement: Composition with registry, Sigil, and Grafana hosted provider

The enrichment module SHALL require no registry changes. It SHALL be usable with `registry.WithLanguageModelMiddleware` because registry already accepts `middleware.Middleware` values.

The enrichment module SHALL document middleware ordering with Sigil. When enrichment appears before `sigil.Stack(...)` in the middleware slice, Sigil hooks and recording SHALL observe enriched `CallOptions`; when enrichment appears after Sigil, enrichment SHALL be transport-only from Sigil's perspective.

The enrichment module SHALL document Grafana hosted provider usage through provider options, not hosted middleware control headers. The recommended Grafana sidecar shape SHALL be `Options{ProviderOptions: ProviderOptionsConfig{ProviderKey: "grafana", ObjectKey: "enrichment"}}`. Enrichment SHALL NOT reinterpret or modify `grafana.sigil`, `grafana.tracing`, `grafana.metrics`, or `grafana.usage` unless callers explicitly configure provider-options fields to those exact names.

#### Scenario: Registry applies enrichment to resolved model

- **WHEN** a provider registry is configured with `registry.WithLanguageModelMiddleware(enrichment.Middleware(opts))`
- **THEN** every model resolved by that registry SHALL receive enrichment behavior

#### Scenario: Enrichment before Sigil is visible to Sigil

- **WHEN** a model is wrapped with enrichment middleware before `sigil.Stack(...)`
- **THEN** subsequent Sigil middleware SHALL observe the enriched call options

#### Scenario: Enrichment after Sigil is transport-only for Sigil

- **WHEN** a model is wrapped with Sigil middleware before enrichment middleware
- **THEN** Sigil middleware SHALL observe the original call options and the inner provider SHALL observe enriched call options

#### Scenario: Grafana hosted controls remain separate

- **WHEN** enrichment writes to `grafana.enrichment`
- **THEN** Grafana hosted middleware controls under `grafana.sigil`, `grafana.tracing`, `grafana.metrics`, and `grafana.usage` SHALL remain separate and unchanged

### Requirement: Validation and documentation for safe use

The implementation SHALL include unit tests for value collection, context defensive copies, default-deny filtering, per-output selection isolation, sensitive redaction/drop behavior, cardinality filtering, over-limit value dropping, generate and stream transformation, header conflict policies, protected headers including absent protected targets, provider-options creation and merge behavior, Grafana field preservation, registry composition, and middleware ordering examples.

The package godoc SHALL document that enrichment is opt-in, default-deny, string-only, and provider-agnostic. It SHALL warn against propagating secrets, API tokens, auth claims without explicit filtering, prompts, tool arguments, raw user input, and high-cardinality metric labels. It SHALL state that the module does not emit telemetry and does not change provider/UI wire behavior unless callers explicitly attach it to a model.

#### Scenario: Unit tests cover generate and stream calls

- **WHEN** the enrichment module test suite runs
- **THEN** it SHALL verify that both `DoGenerate` and `DoStream` receive enriched call options when configured

#### Scenario: Documentation warns about sensitive data

- **WHEN** a consumer reads the package documentation
- **THEN** it SHALL explain the default-deny model and warn not to propagate secrets, tokens, prompts, tool arguments, or raw user input

#### Scenario: No default conformance fixture changes are required

- **WHEN** this module is added without wrapping any model by default
- **THEN** existing root provider-wire and UI/SSE conformance fixtures SHALL remain unchanged
