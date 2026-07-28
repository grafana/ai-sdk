## 1. Module skeleton

- [x] 1.1 Create `middleware/enrichment/go.mod` with module path `github.com/grafana/ai-sdk/middleware/enrichment`, `replace github.com/grafana/ai-sdk => ../../`, and root ai-sdk dependency.
- [x] 1.2 Add package files `doc.go`, `enrichment.go`, `context.go`, `options.go`, `filter.go`, `headers.go`, `provider_options.go`, and package tests.
- [x] 1.3 Add test dependencies only where needed and run `go mod tidy` inside `middleware/enrichment`.
- [x] 1.4 Verify production imports depend only on stdlib plus `github.com/grafana/ai-sdk/middleware` and `github.com/grafana/ai-sdk/provider`.

## 2. Public API and middleware construction

- [x] 2.1 Define typed string enums `Cardinality` and `ConflictPolicy` with all spec-required constants.
- [x] 2.2 Define `Value`, `ValueOption`, `CallInput`, `DynamicValuesFunc`, `FilterOptions`, `HeaderOptions`, `ProviderOptionsConfig`, `Redactor`, `RedactorFunc`, `DefaultRedactor`, and `Options`.
- [x] 2.3 Implement option normalization defaults for conflict policies, filter bounds, and redaction placeholder behavior.
- [x] 2.4 Implement `Middleware(opts Options) middleware.Middleware` using `TransformParams` only.
- [x] 2.5 Implement `Wrap(base provider.LanguageModel, opts Options) provider.LanguageModel` as a convenience wrapper using core middleware utilities.
- [x] 2.6 Add API smoke tests that compile-reference the spec-mandated public symbols and confirm unplanned generic `Source`/`Sink`/`Stack` APIs are not required.

## 3. Value inputs and context helpers

- [x] 3.1 Implement `ValueOption` helpers `Sensitive()` and `WithCardinality(Cardinality)` for values created by `WithValue`.
- [x] 3.2 Implement `WithValue`, `WithValues`, and `ValuesFromContext` using unexported context key types and defensive copies.
- [x] 3.3 Collect static `Options.Values` with defensive copies.
- [x] 3.4 Collect context helper values only when `Options.ContextValues` is true.
- [x] 3.5 Invoke `Options.DynamicValues` with `CallInput` containing call type, current params, and model.
- [x] 3.6 Add unit tests for static values, context opt-in/opt-out, defensive copies, and dynamic value input metadata.

## 4. Filtering and error handling

- [x] 4.1 Implement collection from all configured value inputs with dynamic value errors handled consistently with the spec.
- [x] 4.2 Implement default-deny per-output filtering: `Options.Filter.Include` keys are available to both outputs, `Options.Headers.Map` keys are available only to headers, and `Options.ProviderOptions.Map` keys are available only to provider options.
- [x] 4.3 Implement `Options.Filter.Exclude` precedence over include and output mappings.
- [x] 4.4 Implement invalid/empty key dropping and documented non-zero default `Options.Filter.MaxValueLength` enforcement that drops post-redaction over-limit values without output emission.
- [x] 4.5 Implement `Redactor`, `RedactorFunc`, and `DefaultRedactor()` transformation/drop/sensitive-marking behavior.
- [x] 4.6 Implement sensitive value handling: drop by default, redact when `Options.Filter.RedactSensitive` is true.
- [x] 4.7 Implement `Options.Filter.DropHighCardinality` behavior for `CardinalityHigh`.
- [x] 4.8 Implement default fail-closed dynamic value/output error behavior and `Options.OnError(ctx, err)` override semantics.
- [x] 4.9 Add table-driven unit tests for default deny, include/exclude, per-output selection isolation, redaction/drop, cardinality, over-limit value dropping, and `OnError`.

## 5. Header output

- [x] 5.1 Implement `HeaderOptions` with `Map`, `Prefix`, `Conflict`, and `AdditionalProtected` fields.
- [x] 5.2 Apply `Options.Headers` with copy-on-write headers when `Map` or `Prefix` is configured.
- [x] 5.3 Implement explicit map mode and prefix mode; ensure map keys act as header-only allowlist keys and prefix mode relies on global include filtering.
- [x] 5.4 Implement case-insensitive conflict detection with canonical HTTP header names.
- [x] 5.5 Implement `ConflictCallerWins`, `ConflictEnrichmentWins`, and `ConflictError` behavior.
- [x] 5.6 Implement built-in protected auth/transport header set plus additional protected names, with protected headers never written or overwritten by enrichment by default and no initial opt-in for built-in protected names.
- [x] 5.7 Add unit tests for caller-wins default, all conflict policies, case-insensitive conflicts, protected headers including absent protected targets, prefix mode, and original map immutability.

## 6. Provider options output

- [x] 6.1 Implement `ProviderOptionsConfig` with `ProviderKey`, `ObjectKey`, `Map`, and `Conflict` fields.
- [x] 6.2 Apply `Options.ProviderOptions` with copy-on-write provider options when `ProviderKey` is non-empty.
- [x] 6.3 Implement absent-provider-key creation as `provider.RawProviderOption` JSON.
- [x] 6.4 Implement existing raw-object merge with unrelated field preservation.
- [x] 6.5 Implement existing typed-option merge by marshaling to JSON and storing the merged result as `provider.RawProviderOption`.
- [x] 6.6 Implement nested `ObjectKey` merge and top-level merge when `ObjectKey` is empty.
- [x] 6.7 Implement provider-options `Map` behavior from enrichment key to JSON field name, with globally included values not present in the map using their original keys.
- [x] 6.8 Implement field conflict handling for caller-wins, enrichment-wins, and error.
- [x] 6.9 Add unit tests for absent key creation, raw merge, typed merge, non-object conflict behavior, nested object creation, mapped JSON field names, `provider.ResolveOption` compatibility, and copy-on-write maps.
- [x] 6.10 Add a Grafana-focused unit test proving `sigil`, `tracing`, `metrics`, and `usage` fields are preserved when writing `grafana.enrichment` through `Options.ProviderOptions`.
- [x] 6.11 Add a multi-output unit test proving a key selected only by `Options.Headers.Map` is not emitted by `Options.ProviderOptions` unless also globally included or selected by `Options.ProviderOptions.Map`.

## 7. Middleware integration behavior

- [x] 7.1 Add capture-model tests proving `DoGenerate` receives enriched headers/provider options.
- [x] 7.2 Add capture-model tests proving `DoStream` receives enriched headers/provider options.
- [x] 7.3 Add tests proving prompts, messages, tools, tool arguments, provider metadata, stream parts, and UI chunks are not mutated by enrichment.
- [x] 7.4 Add tests proving original `provider.CallOptions.Headers` and `ProviderOptions` maps are not mutated in place.
- [x] 7.5 Add a registry composition test using `registry.WithLanguageModelMiddleware`.
- [x] 7.6 Add a middleware ordering test or example demonstrating enrichment before and after another middleware.

## 8. Documentation

- [x] 8.1 Write `middleware/enrichment/doc.go` with package overview, API examples, default-deny behavior, privacy warnings, and non-goals.
- [x] 8.2 Document Grafana hosted provider usage with `Options{ProviderOptions: ProviderOptionsConfig{ProviderKey: "grafana", ObjectKey: "enrichment"}}` and preservation of hosted middleware controls.
- [x] 8.3 Document Sigil ordering guidance and clarify that enrichment does not replace Sigil context helpers.
- [x] 8.4 Document `Options.DynamicValues` examples for explicit HTTP request extraction without forwarding all inbound headers.
- [x] 8.5 Update user-facing docs only if maintainers want a guide link for the new opt-in module; avoid root README bloat.

## 9. Build and validation

- [x] 9.1 Run `gofmt` on the new module.
- [x] 9.2 Run `cd middleware/enrichment && go test ./...`.
- [x] 9.3 Run `go test ./...` from the root module.
- [x] 9.4 Run `mise run validate-parity-baseline`.
- [x] 9.5 If aggregate build/test tooling is updated to include the new nested module, run the corresponding aggregate task.
- [x] 9.6 Verify no provider/UI conformance fixtures changed unless a targeted test intentionally requires them.
