## Context

`provider.LanguageModel` already receives `context.Context` on every `DoGenerate` and `DoStream` call, and middleware hooks receive the same context (`provider/language_model.go:10-24`, `middleware/middleware.go:19-51`). The provider-bound request metadata channels also already exist: `provider.CallOptions.Headers` and `provider.CallOptions.ProviderOptions` (`provider/language_model.go:29-48`). Root options populate those fields (`options.go:254-269`) and `StreamText` forwards them into each model step (`streamtext.go:386-403`).

The missing piece is not a new root orchestration field or provider wire format. It is an opt-in middleware module that safely projects explicitly-approved server request context into those existing provider-bound fields. `middleware.Middleware.TransformParams` is the right seam because it receives call type, params, and model before the provider call, and its errors already stop the model call (`middleware/middleware.go:19-24`, `middleware/middleware.go:138-191`).

Important constraints from existing code and specs:

- `PrepareStepResult.Context` is tool-execution state, not provider metadata (`text.go:223-242`). Enrichment must not use or redefine it.
- `middleware.WrapLanguageModel` makes the first middleware outermost and applies `TransformParams` in input order (`middleware/middleware.go:54-58`, `middleware/middleware.go:75-90`). Ordering guidance matters for Sigil composition.
- `provider.ProviderOptions` intentionally round-trips typed values as raw JSON at wire boundaries, and `ResolveOption` recovers typed values from `RawProviderOption` (`provider/provider_option.go:15-120`). Merging enrichment into provider options must preserve unrelated fields and remain compatible with `ResolveOption`.
- `registry.WithLanguageModelMiddleware` already wraps every resolved model (`registry/registry.go:19-25`, `registry/registry.go:54-64`), so no registry changes are needed.
- Sigil is already a nested middleware module and establishes the opt-in module convention under `middleware/<name>/` (`docs/guides/middleware.md:27-37`, `middleware/sigil/doc.go:39-48`).
- Grafana hosted provider controls ride in `ProviderOptions["grafana"]`, not new headers (`providers/grafana/options.go:91-123`, `docs/providers/grafana-cloud.md:88-135`). The provider also overwrites required transport/auth headers after applying caller headers (`providers/grafana/model.go:157-174`), but enrichment should still protect those header names before the provider sees them.
- Upstream `ai@7.0.11` runtime/tool context telemetry filtering is relevant only as a privacy principle: context filtering is shallow and default-deny. This module must not add root `runtimeContext`, `toolsContext`, telemetry spans, or conformance fixtures.

## Goals / Non-Goals

**Goals:**

- Add `middleware/enrichment` as an independent nested Go module with no provider-module imports.
- Provide a logger-style concrete `Options` API for provider-agnostic context enrichment, with explicit value inputs, filter options, and header/provider-options outputs.
- Use `TransformParams` only; support both generate and stream calls without wrapping responses.
- Emit enrichment only into `provider.CallOptions.Headers` and/or `ProviderOptions`.
- Default-deny: no context key is propagated unless allowed by `Options.Filter.Include` or a destination mapping.
- Preserve caller/provider-owned data through deterministic merge and conflict policies.
- Copy maps before mutation so caller-provided `Headers` and `ProviderOptions` are not mutated in place.
- Keep Grafana, Sigil, registry, and future telemetry interactions documented and safe.

**Non-Goals:**

- No prompt, message, tool, tool-argument, `ProviderMetadata`, stream-part, UI/SSE, or telemetry mutation.
- No root package API changes and no new `StreamText` runtime/tool context API.
- No implicit propagation of arbitrary `context.Context` values, all HTTP headers, environment variables, OTel baggage, auth tokens, prompts, raw user input, or secrets.
- No provider-specific imports such as `providers/grafana` or `providers/anthropic`.
- No new conformance fixtures for default root/provider/UI behavior, because the module is opt-in.
- No first-class HTTP request source or Grafana-specific convenience sink in the initial API; callers can express those via `Options.DynamicValues` and `Options.ProviderOptions`.

## Decisions

### D1: Nested module under `middleware/enrichment/`

Create a new module:

```text
middleware/enrichment/
  go.mod
  doc.go
  enrichment.go
  context.go
  source.go
  filter.go
  sink.go
  merge.go
  *_test.go
```

`go.mod` uses `module github.com/grafana/ai-sdk/middleware/enrichment`, `replace github.com/grafana/ai-sdk => ../../`, and depends on the root module. The production module should stay stdlib + root only; tests may add `github.com/stretchr/testify`.

**Rationale:** This follows the documented nested middleware pattern without adding anything to root `go.mod` for consumers that do not import enrichment. It also keeps provider modules out of the dependency graph.

**Alternatives considered:**

- Root `middleware` package: rejected because the feature is optional and broad enough to deserve an isolated import path.
- `middleware/context`: rejected because it conflicts with the Go standard library name and implies future root runtime context parity, while this module is a projection/enrichment layer.

### D2: Public API shape mirrors the logger module style

The package exports a concrete options surface, not a generic source/sink pipeline:

```go
func Middleware(opts Options) middleware.Middleware
func Wrap(base provider.LanguageModel, opts Options) provider.LanguageModel

type Options struct {
    Values          []Value
    ContextValues   bool
    DynamicValues   DynamicValuesFunc
    Headers         HeaderOptions
    ProviderOptions ProviderOptionsConfig
    Filter          FilterOptions
    Redactor        Redactor
    OnError         func(ctx context.Context, err error) error
}

type Value struct {
    Key         string
    Value       string
    Sensitive   bool
    Cardinality Cardinality
}

type ValueOption func(*Value)
func Sensitive() ValueOption
func WithCardinality(cardinality Cardinality) ValueOption

type CallInput struct {
    Type   middleware.CallType
    Params provider.CallOptions
    Model  provider.LanguageModel
}

type DynamicValuesFunc func(ctx context.Context, input CallInput) ([]Value, error)

type FilterOptions struct {
    Include             []string
    Exclude             []string
    RedactSensitive     bool
    DropHighCardinality bool
    MaxValueLength      int
}

type HeaderOptions struct {
    Map                 map[string]string
    Prefix              string
    Conflict            ConflictPolicy
    AdditionalProtected []string
}

type ProviderOptionsConfig struct {
    ProviderKey string
    ObjectKey   string
    Map         map[string]string
    Conflict    ConflictPolicy
}

type Redactor interface {
    RedactValue(ctx context.Context, value Value) (Value, bool)
}
type RedactorFunc func(ctx context.Context, value Value) (Value, bool)
func DefaultRedactor() Redactor

type Cardinality string
const (
    CardinalityLow     Cardinality = "low"
    CardinalityBounded Cardinality = "bounded"
    CardinalityHigh    Cardinality = "high"
)

type ConflictPolicy string
const (
    ConflictCallerWins     ConflictPolicy = "caller_wins"
    ConflictEnrichmentWins ConflictPolicy = "enrichment_wins"
    ConflictError          ConflictPolicy = "error"
)
```

Context helpers remain package-level functions:

```go
func WithValue(ctx context.Context, key, value string, opts ...ValueOption) context.Context
func WithValues(ctx context.Context, values ...Value) context.Context
func ValuesFromContext(ctx context.Context) []Value
```

`Options.Values` provides static values. `Options.ContextValues` opts into values stored through the context helpers. `Options.DynamicValues` is the caller extension point for request-derived values such as HTTP request IDs, auth-claim projections, tenant configuration, or feature flags.

`Options.Headers` and `Options.ProviderOptions` are disabled when their zero values do not name a destination: headers require either `Map` or `Prefix`; provider options require `ProviderKey`. The package does not expose `Source`, `Sink`, `Static`, `FromContext`, `HeaderSink`, `ProviderOptionsSink`, or `Stack` in the initial API.

**Rationale:** This follows the `middleware/logger` proposal style: one `Options` struct with concrete sub-options, a callback for dynamic per-request data, a redactor interface/func pair plus `DefaultRedactor()`, and `Middleware`/`Wrap` entry points. It keeps common use sites readable while still preserving explicit collection, filtering, copy-on-write, and merge rules.

**Alternatives considered:**

- Generic `Source` and `Sink` interfaces: flexible, but too framework-like for the initial public API and less consistent with the logger middleware's concrete options model.
- A single callback that mutates `CallOptions`: rejected because it would force every caller to reimplement filtering, redaction, copy-on-write, and merge rules.
- `Stack(opts) []middleware.Middleware`: rejected for consistency with logger; callers can pass `enrichment.Middleware(opts)` directly to `middleware.WrapLanguageModel` or `registry.WithLanguageModelMiddleware`.
- Provider-specific helpers such as `GrafanaProviderOptions()` in v1: rejected for now to keep the module provider-agnostic and avoid freezing backend naming beyond a documented `Options.ProviderOptions` example.

### D3: Value collection is explicit and default-deny

The middleware collects values only from configured `Options.Values`, `Options.ContextValues`, and `Options.DynamicValues`. Context collection reads only values stored by `enrichment.WithValue`/`WithValues` using unexported context key types and defensive copies, following the collision-safe pattern in `middleware/sigil/context.go:9-14` and `middleware/sigil/context.go:84-99`.

Filtering is shallow and string-only. The middleware normalizes collected values once, then derives a filtered value slice for each destination:

1. copy static `Options.Values`;
2. append context helper values only when `Options.ContextValues` is true;
3. append values returned by `Options.DynamicValues`, passing `CallInput` with call type, current params, and model;
4. drop invalid/empty keys;
5. apply `Options.Redactor`, defaulting nil to `DefaultRedactor()`, dropping values when it returns `false`;
6. drop sensitive values unless `Options.Filter.RedactSensitive` is true, in which case the value string is replaced with a documented redaction placeholder unless the redactor already changed it;
7. drop values whose post-redaction string length exceeds the documented non-zero default `Options.Filter.MaxValueLength`, or the caller-provided override;
8. drop `CardinalityHigh` values when `Options.Filter.DropHighCardinality` is true;
9. for each destination, emit only values selected for that destination: keys in `Options.Filter.Include` are available to both headers and provider options, `Options.Headers.Map` selects keys only for headers, `Options.ProviderOptions.Map` selects keys only for provider options, and `Options.Filter.Exclude` wins over all selectors.

Prefix header mode requires `Options.Filter.Include`; it does not select every collected value by itself. If `Options.ProviderOptions.Map` is empty, provider options receive only globally included values and write them under their original keys. If `Options.ProviderOptions.Map` is set, map values become JSON field names for provider options. `DefaultRedactor()` marks known secret-looking key names as sensitive and otherwise leaves values unchanged, so the normal sensitive-value handling decides whether those values are dropped or emitted as the redaction placeholder.

**Rationale:** This borrows the upstream telemetry filtering principle from the registered `ai@7.0.11` baseline (shallow, explicit include only) without adding upstream runtime context APIs. String-only values avoid nested object filtering and accidental prompt/user-data leakage.

**Alternatives considered:**

- Iterating arbitrary Go context keys: impossible and intentionally unsupported by `context.Context`.
- First-class HTTP request source: deferred. HTTP extraction is easy to write as `Options.DynamicValues`, and a built-in source could encourage blanket inbound-header propagation.
- Default dropping all high-cardinality values: rejected for header/provider option outputs because request IDs and trace IDs are legitimate correlation/routing values. Future telemetry outputs should default to `DropHighCardinality: true`.

### D4: Error policy is fail-closed by default, with explicit override

If dynamic value collection or header/provider-options application returns an error, `Middleware.TransformParams` fails the model call by default. If `Options.OnError` is set, the middleware calls it with the context and error; a non-nil return replaces the error, and a nil return suppresses the error so the request proceeds with the last successfully built params.

**Rationale:** Routing, audit, and tenant-enrichment failures should not silently disappear. Observability-only callers can opt into fail-open behavior explicitly.

**Alternatives considered:**

- Always fail-open: rejected because silently losing enrichment can break routing/audit assumptions.
- Separate callbacks for collection and destination errors: rejected for initial API simplicity.

### D5: Header options use canonical conflict detection and protected headers

`Options.Headers` writes into a copied `params.Headers` map when either `Map` or `Prefix` is configured. It supports explicit map mode (`enrichment key -> HTTP header name`) and prefix mode (`Prefix + key`). Explicit maps also act as destination-specific allowlists for headers only; a key selected by `Options.Headers.Map` is not emitted to provider options unless it is also globally included or selected by `Options.ProviderOptions.Map`. Prefix mode requires `Options.Filter.Include` to avoid propagating all collected values.

Conflict detection is case-insensitive using canonical HTTP header names. Default conflict policy is caller-wins:

- `ConflictCallerWins`: preserve existing caller header.
- `ConflictEnrichmentWins`: overwrite existing caller header, unless the target is protected.
- `ConflictError`: return an error.

Protected headers are never written or overwritten by enrichment, regardless of conflict policy. If enrichment targets a protected header and the caller did not already set it, the header is still omitted from enrichment output. The built-in protected set includes auth and transport headers such as `Authorization`, `Proxy-Authorization`, `X-Access-Token`, `X-Grafana-Id`, `Content-Type`, provider API-key headers, and Grafana provider wire headers. `HeaderOptions.AdditionalProtected` extends the built-in set for deployment-specific names; the initial API does not provide an opt-in to write built-in protected headers.

**Rationale:** This matches the existing caller-wins map merge precedent in `middleware/default_settings.go:73-92` while adding header-specific case normalization and auth/transport safety.

**Alternatives considered:**

- Let enrichment write or overwrite protected headers with `ConflictEnrichmentWins`: rejected because auth and provider transport identity should be configured through provider-specific APIs, not context enrichment.
- Case-sensitive conflict detection: rejected because HTTP header names are case-insensitive.

### D6: Provider options config shallow-merges JSON and preserves unrelated fields

`Options.ProviderOptions` writes into a copied `params.ProviderOptions` map when `ProviderKey` is non-empty. It creates or merges JSON objects using the existing `provider.RawProviderOption` mechanism. Values selected by `Options.ProviderOptions.Map` write to the corresponding JSON field names; globally included values not present in the map write under their original keys. Values selected solely by `Options.Headers.Map` are not visible to provider options.

1. If the provider key is absent, create `provider.RawProviderOption{Key: ProviderKey, Raw: <object JSON>}`.
2. If present, marshal the existing typed or raw option to JSON.
3. Require existing JSON to be an object. Non-object conflicts obey `ConflictPolicy`.
4. If `ObjectKey` is empty, shallow-merge enrichment fields into the top-level object.
5. If `ObjectKey` is set, shallow-merge into that nested object, creating it when absent.
6. Preserve unrelated existing fields.
7. Store the merged result as `provider.RawProviderOption{Key: ProviderKey, Raw: mergedJSON}`.

For Grafana hosted provider usage, docs should recommend:

```go
enrichment.Middleware(enrichment.Options{
    ProviderOptions: enrichment.ProviderOptionsConfig{
        ProviderKey: "grafana",
        ObjectKey:   "enrichment",
    },
    Filter: enrichment.FilterOptions{Include: []string{"request_id", "tenant"}},
})
```

This shape preserves `grafana.sigil`, `grafana.tracing`, `grafana.metrics`, and `grafana.usage` controls while adding a sidecar `grafana.enrichment` object. The destination must never reinterpret or modify those controls unless the caller explicitly chooses those exact keys by configuring `ObjectKey`/value keys accordingly.

**Rationale:** `provider.ProviderOptions` already supports typed/raw lossless JSON (`provider/provider_option.go:34-89`). Storing the merged value as raw JSON preserves wire shape and allows downstream providers to recover typed views via `provider.ResolveOption`.

**Trade-off:** A downstream middleware that performs direct type assertions on `params.ProviderOptions[ProviderKey]` after enrichment may see `RawProviderOption` instead of the original typed struct. This is acceptable because `ResolveOption` is the supported typed recovery API; ordering should be documented for middlewares that inspect direct concrete types.

### D7: Composition and documentation guidance

`enrichment.Middleware(opts)` uses `TransformParams` only. Therefore the existing middleware ordering rules determine who sees enriched params:

- Use `middleware.WrapLanguageModel(model, enrichment.Middleware(opts), sigil.Stack(sigilOpts)...)` when Sigil hooks/recording should see enriched `CallOptions`.
- Put enrichment after Sigil when enrichment is transport-only and should not appear in Sigil evaluation or recording inputs.
- Use `registry.WithLanguageModelMiddleware(enrichment.Middleware(opts))` when every model resolved by a registry should receive the same enrichment.

Docs must also clarify that enrichment is not a replacement for Sigil context helpers, Grafana `WithUserIDToken`, Grafana hosted provider controls, or future core telemetry.

### D8: Validation strategy

Because this module is opt-in and does not change default root/provider/UI wire behavior, implementation does not need broad conformance fixture regeneration. It should include targeted unit tests in `middleware/enrichment` and run:

- `cd middleware/enrichment && go test ./...`
- `go test ./...`
- `mise run validate-parity-baseline`

If implementation changes root test/build tasks to include the new nested module, run the corresponding aggregate task as well.

## Risks / Trade-offs

- [Accidental sensitive propagation] → Default-deny filtering, explicit value inputs, sensitive dropping/redaction, protected headers, and docs warning against auth tokens, prompts, raw user input, and secrets.
- [ProviderOptions typed-to-raw conversion surprises direct type assertions] → Document that middlewares/providers should use `provider.ResolveOption`; preserve JSON losslessly and test typed-to-raw resolution.
- [Header behavior varies by provider] → Keep module-level behavior deterministic and protected; provider-specific transports may still override required headers, as Grafana does in `providers/grafana/model.go:157-174`.
- [Cardinality misuse as metrics labels] → This module emits request metadata only and no metrics; docs warn future telemetry consumers to drop high-cardinality values by default.
- [Open-ended DynamicValues can leak data] → Built-in helpers stay safe and docs show explicit extraction only. No all-header/all-context built-ins.
- [Middleware ordering confusion] → Provide docs and tests demonstrating before/after ordering with another middleware and registry wrapping.

## Migration Plan

This change is additive.

1. Land the OpenSpec proposal.
2. Implement the new nested module and tests under `middleware/enrichment`.
3. Update aggregate test/build tooling only if maintainers want nested-module tasks to include enrichment by default.
4. Consumers opt in by importing `github.com/grafana/ai-sdk/middleware/enrichment` and wrapping models or registries.

Rollback is local: remove the `middleware/enrichment` module and docs/tasks introduced by implementation. Root behavior remains unchanged for consumers that never imported the module.

## Open Questions

No issue-level decisions are left blocking implementation. The proposal intentionally resolves the main open questions as follows:

- No `GrafanaProviderOptions()` helper in v1; use explicit `Options.ProviderOptions` configuration with `ProviderKey: "grafana"` and `ObjectKey: "enrichment"`.
- `enrichment` is the recommended Grafana sidecar object key in documentation, but the provider-options destination remains configurable.
- High-cardinality values are allowed by default for header/provider option outputs and can be dropped with `Filter.DropHighCardinality`.
- Dynamic value/output errors fail closed by default and can fail open through `OnError`.
- HTTP request extraction is documented as `Options.DynamicValues`, not a built-in source.
- Protected headers use a built-in set plus caller-supplied additions; overwriting protected names remains disallowed by default.
