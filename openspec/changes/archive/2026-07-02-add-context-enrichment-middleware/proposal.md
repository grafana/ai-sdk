## Why

Server-side request context already reaches every `provider.LanguageModel` call, and callers can manually attach provider-bound metadata through `CallOptions.Headers` and `ProviderOptions`, but the repo has no reusable, safe middleware for projecting approved request context into those channels. Teams currently have to build ad hoc enrichment logic, which risks inconsistent merge behavior, accidental sensitive propagation, and confusion with tool-only context or Grafana-hosted middleware controls.

This change adds a small opt-in middleware module that makes context enrichment explicit, provider-agnostic, default-deny, and composable with the existing middleware, registry, Sigil, and Grafana provider option shapes.

## What Changes

- Add a new nested Go module at `middleware/enrichment/` with module path `github.com/grafana/ai-sdk/middleware/enrichment`.
- Export a provider-agnostic enrichment middleware API shaped like the proposed logger module:
  - `Middleware(opts Options) middleware.Middleware` using `TransformParams` only.
  - `Wrap(base, opts) provider.LanguageModel` convenience.
  - Concrete option structs: `Options`, `FilterOptions`, `HeaderOptions`, and `ProviderOptionsConfig`.
  - Value/callback types: `Value`, `ValueOption`, `CallInput`, `DynamicValuesFunc`, `Redactor`, `RedactorFunc`, `DefaultRedactor`, `Cardinality`, and `ConflictPolicy`.
- Provide explicit/default-deny enrichment value inputs through `Options`:
  - static `Options.Values`.
  - typed context helpers `WithValue`, `WithValues`, and `ValuesFromContext`, enabled by `Options.ContextValues`.
  - caller-supplied `Options.DynamicValues` for request-derived values.
- Provide concrete output options for the two existing provider-bound metadata channels:
  - `Options.Headers` for `CallOptions.Headers`.
  - `Options.ProviderOptions` for `CallOptions.ProviderOptions` JSON objects.
- Define deterministic filtering, redaction, cardinality, length-limit, and conflict behavior.
- Define protected header behavior so enrichment cannot write or overwrite auth/transport headers by default.
- Document composition with `registry.WithLanguageModelMiddleware`, Sigil ordering, and Grafana hosted provider controls.
- Do not change root `middleware`, root `aisdk.StreamText` APIs, provider wire protocol, UI/SSE chunks, conformance fixtures, or any provider implementation by default.

## Capabilities

### New Capabilities

- `context-enrichment-middleware`: Opt-in nested middleware module for collecting explicitly-approved request context values and projecting them into `provider.CallOptions.Headers` and/or `provider.CallOptions.ProviderOptions` with safe filtering and deterministic merge semantics.

### Modified Capabilities

(none — existing `language-model-middleware`, `provider-registry`, `typed-provider-options`, `sigil-middleware`, and `grafana-provider-options` requirements remain unchanged; this proposal adds a sibling module that composes with those capabilities.)

## Impact

- **New module:** `middleware/enrichment/` with its own `go.mod`, depending on the root module and the standard library. Tests may use `testify`; no provider module imports are required.
- **Root module:** unchanged. Consumers who do not import `github.com/grafana/ai-sdk/middleware/enrichment` pay no dependency or API cost.
- **Public API:** additive package-level API in the new module only.
- **Provider calls:** opt-in middleware mutates only copies of `provider.CallOptions.Headers` and `ProviderOptions` at the existing `TransformParams` seam before `DoGenerate`/`DoStream`.
- **Wire/conformance:** default behavior is unchanged. Because the module is opt-in and does not alter provider/UI wire formats, no broad conformance fixture regeneration is expected.
- **Docs/tests:** add package godoc and targeted unit tests for value inputs, filters, header/provider-options outputs, merge rules, registry composition, and ordering examples.
