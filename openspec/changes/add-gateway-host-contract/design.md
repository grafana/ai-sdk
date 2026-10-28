# Design

## Context

After #481 (`nrbrd/gateway-execution-runtime`, #555), the runtime is shared, but composition is not:

- `gateway.go` builds two runtimes and two ProviderWire handlers: configured, and BYOK in `cloud` mode. `routes.go` dispatches between them with `byAccess`.
- `service/selection.go` has two selectors, plus `ConfiguredDiscovery` and `BYOKDiscovery` wrappers that make the access decision for discovery.
- `providerv4.HostErrorWriter` is a concrete ProviderWire type that host code imports directly. It still carries an unused authentication category.
- `internal/discovery` owns both the safe listing and the ProviderWire document.
- Native option validation (`nativeoptions.Model`) wraps each configured candidate inside fallback. For BYOK, it wraps the whole fallback chain.
- #248 adds `WithChatDefaults` to each catalog candidate. It reads Chat-specific state from the context and rewrites options for each backend.

The fallback default decider is unexported. It retries on unknown errors and stops only on non-retryable `APICallError`, context-window errors and the buffer limit.

## Goals / Non-Goals

**Goals:**
- Host code depends on adapter interfaces, never on ProviderWire types.
- The production route table is the only place that names ProviderWire.
- Translation is the only seam an adapter has into candidates.

**Non-Goals:**
- Changing selection, classification or the stream loop beyond the translation sentinel.
- A plugin registry, dynamic routes or route options beyond the entry fields.
- Moving ProviderWire's path and envelope validation to the host.

## Decisions

### D1. Host contract package `ai-gateway/internal/host`

It holds what the host and adapters share, apart from execution:
- `ErrorCategory`, a closed set: `ErrorPermission`, `ErrorInternal` and `ErrorBYOKDiscovery`;
- the `ErrorWriter` interface, `WriteHostError(http.ResponseWriter, ErrorCategory)`;
- the `DiscoveryProjection` interface, which turns `[]catalog.ModelInfo` into document bytes.

The adapter contract godoc lives in its `doc.go`.

`providerwire/v4` implements both interfaces and keeps its documents. `HostErrorAuthentication` and the host mapping to `safeAuthentication` are removed. `safeAuthentication` stays only if a ProviderWire path still uses it.

- *Alternative: put the contract in `execution`.* Rejected: the runtime never writes HTTP responses, and host errors do.
- *Alternative: keep it under `cmd/`.* Rejected: `providerwire/v4` cannot import `cmd/.../internal`.

### D2. One access-aware selector

`service.NewSelector` takes the catalog resolver and, in `cloud` mode only, the BYOK client and factory. It reads the verified caller:
- `ConfiguredAccounts` delegates to `execution.CatalogSelector`;
- `RequestBYOK`, with BYOK configured, decodes and builds the request model as `NewBYOKSelector` does today;
- anything else returns `execution.ErrAccountAccess`.

`gateway.go` builds one runtime and one ProviderWire handler. `routeHandlers`, `byAccess`, `ConfiguredDiscovery` and `BYOKDiscovery` are removed.

- *Alternative: keep two runtimes behind a host dispatcher.* Rejected: every adapter would need one handler per access mode.

### D3. Route table in `cmd/.../gateway`

A `route` is `{method, path, label, handler, errors host.ErrorWriter}`. `newHandler(policy, telemetry, routes)`:
- validates the table: labels, handlers, writers and unique method and path pairs;
- registers each entry on one `ServeMux` behind `AccessMiddleware`, which refuses with that entry's writer;
- answers `HEAD` on each `GET` entry with the current 405, without work;
- labels observations from the matched entry, or `unmatched`.

`productionRoutes` returns the two entries. A test compares it with the manifest operations, reusing the `operations` helper from the manifest test. The ProviderWire language-model entry keeps its path normalization, wrapped at the entry, so `providerwire/v4` keeps its own path check for standalone use.

- *Alternative: validate against the manifest at startup.* Rejected: tests register an extra route. The platform never routes undeclared paths, so a test-time check gives the same guarantee.

### D4. Host-owned discovery

`internal/discovery` keeps the safe listing. `discovery.New(lister, projection, errors)` decides by access:
- configured lists the catalog;
- BYOK writes `ErrorBYOKDiscovery`;
- anything else writes `ErrorPermission`.

A panic, a listing error or a projection error writes `ErrorInternal`. A successful projection is written as `200 application/json`. The document types, their sorting, encoding and `schema.json`, together with their tests, move to `providerwire/v4/discovery.go`.

- *Alternative: the projection writes the response.* Rejected: a failed projection would leave a partial response, and the host could not write the internal error.
- *Alternative: the projection returns a content type.* Deferred: every planned adapter serves JSON discovery.

### D5. Per-attempt translation in `execution`

Types:
- `Target{Type TargetType, ProviderName string, ModelID string}`, where `TargetType` is a typed enum: `anthropic`, `openai` or `openai-compatible`. `ProviderName` is the compatible provider name that #248 needs for its namespace.
- `Translator func(Target, provider.CallOptions) (provider.CallOptions, error)`.

Flow:
- `Runtime.Call` takes an optional `WithTranslator(t)` and stores the translator in the call context, next to the attempt observer. A request with no translator stores nothing.
- `execution.Translate(model, target)` returns a wrapper. On each `DoGenerate` or `DoStream`:
  - with no translator in the context, it delegates unchanged;
  - otherwise it clones the top-level `ProviderOptions` and `Headers` maps and calls the translator under `recover`;
  - it re-checks the result with the runtime's policy: no call-level `gateway` namespace and no protected header;
  - then it delegates.
- Translators must not mutate nested values. This is documented on `Translator`.
- A panic or a policy refusal returns `ErrTranslation`, which never carries values. `ClassifyProvider` checks it first and returns internal. A translator's own error is returned as-is.

Wrapping order for every candidate is native → `nativeoptions.Model` (when the type validates) → `Translate` → fallback. Single-candidate configured models are wrapped too. In `byok.New`, each account is wrapped in `nativeoptions.Model` and then `Translate`, and the outer Anthropic validation wrapper is removed.

Stopping fallback on `ErrTranslation` requires a decider that honors it. The root module exports `fallback.DefaultDecider`. Gateway chains use `WithDecider` to stop on `ErrTranslation` and otherwise delegate to the default. The cost is an additive root API with a test, and no behavior change.
- *Alternative: a Gateway decider that copies the default rules.* Rejected: it duplicates the context-window heuristic, which would drift.
- *Alternative: make `ErrTranslation` look like a non-retryable `APICallError`.* Rejected: attempt summaries would then report a provider failure that never happened.

- *Alternative: translate once, before fallback.* Rejected: a heterogeneous chain needs different native options for each target.
- *Alternative: translation inside native validation.* Rejected: validation must see the translated options.

### D6. Test adapter as a test-only package

`lineAdapter` moves from `execution_test` to `ai-gateway/internal/testadapter`. It becomes an `http.Handler` over a runtime, with a plain-text line format for outcomes and host errors, and an optional translator. `execution_test` keeps its parity table over it. Gateway route tests register it on an extra entry. A test asserts that `go list -deps ./cmd/grafana-ai-gateway` excludes the package.

- *Alternative: a second copy in the gateway tests.* Rejected: the parity evidence and the composition evidence would drift apart.

### D7. Docs

- `docs/concepts/gateway-architecture.md`: the request flow names the one selector, the route table and host-owned discovery.
- `docs/concepts/gateway-glossary.md`: adds "host contract", "route table" and "option translator; per-attempt translation". The existing "Adapter; translator" row is reworded so "translator" stays unambiguous.

## Risks / Trade-offs

- [BYOK Anthropic validation now fails each account in turn, because the unsupported-request error is retryable] → There is still zero native I/O and the same document. Attempts are captured but not published (#420). This is recorded in the `gateway-request-byok` delta.
- [A translator can be slow or ignore cancellation] → It runs on the request goroutine under the request context, like the selector. The platform deadline ends the caller's request.
- [Translators that mutate nested option values leak between attempts] → The contract is documented, and the concurrency and heterogeneous-chain tests use translators that write new values.
- [The root `fallback` export rides on a `refactor` PR title, which triggers no release] → The root API is additive, and the Gateway builds from workspace source. Library consumers get it with the next root release.
- [Path normalization stays ProviderWire-specific] → It is confined to the production entry, and new adapters can match their own path.

## Migration Plan

Internal refactor only. There are no operator setting or manifest changes. Rollback is reverting the PR.
