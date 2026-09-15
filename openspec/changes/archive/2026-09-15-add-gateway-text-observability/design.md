## Context

Work package 5 leaves an authenticated service whose catalog stores one direct Anthropic model per canonical public route. The ProviderWire handler resolves aliases to `catalog.ResolvedModel{ID, Model}`, calls the provider, and owns strict unary commitment plus the streaming state machine, timeout, cancellation, part bound, and bounded post-terminal drain. The outer HTTP telemetry records only route health; it does not observe model usage or generation lifecycle.

The repository already has dependency-isolated Apache modules for structured logging, Prometheus model metrics, context-to-provider enrichment, and Agent Observability. They are suitable building blocks, but their direct-provider defaults are wrong for a logical Gateway route: logger and Agent Observability prefer response identity, and Prometheus prefers response identity unless configured otherwise. Prometheus starts an unbounded asynchronous drain after consumer cancellation. In the reconciled main baseline, Agent Observability does not drain after cancellation; positive drain duration adds opt-in finite cleanup while zero preserves that behavior. The logger synchronously summarizes immediately available parts until its input becomes idle, which can delay finalization indefinitely for a continuously ready upstream. The logger also emits some provider response metadata by default. The Gateway must opt into canonical requested identity and an allowlisted logical record without changing those defaults for SDK users.

The change spans the Apache middleware modules and the AGPL service. Apache commits must precede the Gateway commit, and `ai-gateway/go.mod` must use immutable proxy-resolvable pins without committed local replacements or root `go.work` registration.

### Trust and cardinality boundary

| Logical field | Initial source | Destination | Excluded destinations |
| --- | --- | --- | --- |
| caller service | normalized verified `auth.Caller.Service` | logs and Agent Observability metadata | provider options/headers and Prometheus labels |
| namespace | normalized verified `auth.Caller.Namespace` | logs and Agent Observability metadata | provider options/headers and Prometheus labels |
| request correlation | Gateway-generated bounded opaque ID, once per HTTP request | HTTP/model logs and Agent Observability metadata | provider options/headers and Prometheus labels |
| region | validated static deployment setting, when configured | logs and Agent Observability metadata/tag | provider options/headers; no per-request metric label |
| application | validated static deployment setting, when configured | logs and Agent Observability metadata/tag | provider options/headers; no per-request metric label |

Unverified request headers, raw auth claims, acting-user details, tokens, prompts, provider options, and arbitrary context values are never enumerated. Region or application remains absent when no trusted source is configured. The existing `middleware/enrichment` module is intentionally not placed in provider-output mode: WP8 has no approved reason to send these values to Anthropic.

## Goals / Non-Goals

**Goals:**

- Emit one logical logging lifecycle, one logical Prometheus observation, and one Agent Observability generation for every authenticated text model invocation.
- Use provider `grafana` plus the canonical public catalog ID for all logical identity, including calls made through aliases.
- Correlate request completion, model logs, and Agent Observability using bounded approved metadata.
- Preserve ProviderWire bytes, invocation count, stream order, cancellation, and bounded cleanup.
- Make exporter failure non-fatal to model traffic while bounding queueing, payloads, retries, flush, and shutdown.
- Keep direct SDK middleware behavior backward compatible through additive options with unchanged defaults.

**Non-Goals:**

- Physical provider/backend attempt records, retry decisions, or fallback topology (WP9).
- Image capacity and distribution (WP6), the Go client (WP7), production rollout (WP10), or per-request Agent Observability controls (WP27).
- Hooks/preflight policy, prompt transformation, content capture, raw output, tools, files, reasoning content, provider options, or body-carried headers.
- New ProviderWire DTOs, fields, validation, errors, SSE events, or lifecycle rules.
- Making `middleware/enrichment` a telemetry pipeline or forwarding logical context to providers.

## Decisions

### 1. Compose telemetry on catalog entries, not in ProviderWire

`BuildCatalog` will receive a model-observability factory and wrap each canonical catalog entry's logical model once as the entry is constructed. In WP8 that inner model is direct; WP9 may replace it with a fallback model beneath the unchanged logical wrapper without wrapping physical candidates. The wrapper uses `middleware.WrapOptions{ProviderID: "grafana", ModelID: canonicalID}` so every middleware sees canonical public identity even when the request used an alias. The ordered middlewares are:

1. Gateway logical-context bridge;
2. Agent Observability recording only (hooks omitted);
3. structured logger;
4. Prometheus model metrics;
5. canonical identity override around the direct provider.

The concrete wrapping may use one `middleware.Wrap` call, whose first middleware is outermost. Catalog aliases continue returning the same already-composed model pointer. Wrapping in the handler was rejected because it would rebuild middleware per request and make exactly-once behavior and collector registration harder to prove. Adding telemetry inside ProviderWire was rejected because future API adapters must reuse the same logical model boundary.

### 2. Keep logical and physical identity separate

Logger and Agent Observability gain an additive `IdentitySource` option matching the existing Prometheus concept. Their zero value remains response-preferred. Requested mode uses only the wrapped model's provider/model for start and terminal identity and omits response/transport identity fields, including provider response IDs and backend model IDs. Prometheus uses its existing `IdentityRequested` setting.

The Gateway canonical wrapper reports provider `grafana` and the canonical catalog ID. Provider response metadata continues through the unmodified result/stream for ProviderWire processing; only observation mapping ignores it. A Gateway logger allowlist removes raw finish reasons, warning feature strings, response metadata, and any future non-approved attributes while retaining closed outcomes, unified finish, usage, counts, and timing.

Rewriting provider results was rejected because it would mutate shared domain values and could alter ProviderWire streaming metadata. Normalizing backend labels to a constant with existing callbacks was rejected because logger and Agent Observability would still retain transport metadata.

### 3. Enrich an internal observation view, never provider call options

The outer HTTP telemetry creates one opaque request correlation ID and stores it in private request context. After successful authentication, the logical-context bridge reads only `auth.CallerFromContext`, the request correlation value, and validated static region/application settings. One immutable observation view feeds logger dynamic attributes and Agent Observability `ContextInfo`.

The bridge uses fixed metadata keys and length limits. Missing values are omitted. It does not set `CallOptions.Headers`, `CallOptions.ProviderOptions`, Agent Observability `UserID`, or metric labels. This keeps auth and host metadata out of providers and prevents unbounded label cardinality.

Trusting arbitrary incoming correlation/region/application headers was rejected because WP5 proves only the authentication claims. Supporting a configured trusted-proxy header can be proposed later with an explicit trust boundary.

### 4. Default every Gateway logical surface to metadata-only privacy

The model logger uses zero capture flags, disables per-part logging, uses requested identity, and passes every record through a Gateway-owned allowlist followed by the logger's default secret redactor. Agent Observability is constructed with `ContentCaptureModeMetadataOnly`, requested identity, hooks disabled, and an allowlisted `ContextInfo`. Metadata-only mode preserves lifecycle, structure, usage, timing, and closed error classification while stripping prompt/output text and detailed error text.

The pinned Agent Observability v0.15.0 client does not run `GenerationSanitizer` in metadata-only mode. Its start path also reads ambient conversation/user/agent/tag/experiment context and client defaults; an allowlisted `ContextInfo` alone cannot exclude these. The reusable mapper retains raw stop reason and numeric provider-option/usage metadata. Gateway integration therefore uses additive reusable controls for a provided-only recording context and a final consumer-owned generation filter. The filter receives the observed `provider.FinishReason`, retains only the unified closed value plus approved normalized fields, and runs before the client sees the mapped generation. The client subsequently stamps only its fixed SDK provenance/content-capture markers and, on failure, a mirrored closed error category; these are client-owned protocol metadata rather than request enrichment. A nil filter preserves direct-consumer behavior, and a panic produces a minimal safe generation while the model call remains fail-open. The Gateway also rejects the exact SDK environment layer before client construction. Requested identity and content capture alone do not satisfy the privacy acceptance criteria; the reusable middleware keeps its existing defaults unchanged.

The Agent Observability client is process-wide and optional only when its exporter is explicitly disabled for development/tests. Initial production configuration in WP10 must enable it. Export settings are represented by strict Gateway settings and secret environment-variable references; they are validated before client construction. The exact-pinned exporter receives finite queue, batch, payload, retry/backoff, and shutdown budgets. Export failures and queue pressure increment a bounded metric and emit a fixed diagnostic class, never the exporter error or configuration, and never fail the model call.

Full or per-request content capture was rejected for WP8 because the acceptance contract requires payload privacy and WP27 owns per-request controls.

### 5. Preserve stream ownership with one bounded cleanup owner per layer

Logger, Prometheus, and Agent Observability each gain an optional positive stream-drain duration. On downstream cancellation their tee owner closes its output exactly once, finalizes one observation, and drains only its immediate upstream until channel close or its own absolute deadline. Zero retains the existing direct-SDK behavior. The Gateway always configures the same finite drain duration already validated for ProviderWire.

The ProviderWire handler remains the owner of its input stream and protocol terminal rules. Each middleware owns only the channel it introduced; no middleware emits, suppresses, reorders, or edits parts. This allows cancellation to cascade through nested tees without an indefinitely retained Gateway-owned drain goroutine.

Removing drains entirely was rejected because it can strand a cooperative producer during cancellation races. Sharing one cross-package drain goroutine was rejected because it couples otherwise independent Apache modules and blurs channel ownership.

WP7 needs no client API or wire change for this design. Correlation is generated and consumed inside the server and is neither accepted from nor returned to either Gateway client.

### 6. Reuse the service Prometheus registry without mixing HTTP and model cardinality

`Telemetry` exposes its private registry through a narrow registerer method used during startup. The Prometheus model middleware registers once there. Its labels stay limited to operation, canonical configured public model, closed status/error class/status code/finish reason, token type, and closed stream part type. Caller, namespace, correlation, provider/backend identity, and arbitrary values are forbidden labels.

The existing `/metrics` route remains unchanged. Duplicate collector registration is a startup error before readiness. Creating a second registry or endpoint was rejected because operators need one scrape target.

### 7. Align lifetime with the process

All model middleware and the Agent Observability client are constructed before listener binding and readiness. On shutdown, HTTP readiness falls first and request contexts are canceled as WP5 specifies. Because an abandoned stream's observer goroutine can outlive its HTTP handler, the recorder emits an exact-once post-End completion signal. The process refuses new recorder acquisition after close begins and boundedly waits for the active set before starting a fresh bounded flush and shutdown. Flush failure is reported only as a fixed telemetry-shutdown class and does not expose configuration or generation data.

## Risks / Trade-offs

- **[Nested tees add buffering and goroutines]** → Keep each buffer finite, preserve the existing 64-part module defaults, test continuously-ready and silent streams, and bound every cancellation drain.
- **[Alias-specific usage becomes invisible]** → This is intentional: logical telemetry is canonical. The generated request correlation ID supports per-request debugging without making aliases or callers metric labels.
- **[Metadata-only Agent Observability reduces demo richness]** → It still proves generation lifecycle, usage, timing, correlation, and canonical identity. Richer capture needs an explicit privacy decision and belongs to WP27 or a separate change.
- **[Exporter outages can lose records]** → Keep model traffic fail-open, bound queues/retries, count dropped/export-failed records, and make WP10 smoke verify successful export in the target environment.
- **[Static region/application can be missing]** → Omit rather than invent or trust request input; WP10 production configuration supplies validated values if operators require them.
- **[Middleware additions span three modules]** → Keep them additive and limited to identity selection and bounded drain behavior; all service-specific policy remains under `ai-gateway/`.

## Migration Plan

1. Land additive Apache middleware changes and publish or identify immutable proxy-resolvable versions.
2. Pin those versions in `ai-gateway/go.mod`, add Gateway context/config/factory composition, and verify with `GOWORK=off` and no `replace` directive.
3. Run deterministic in-process and real-command tests with a capturing logger, isolated Prometheus registry, fake Agent Observability exporter, and fake Anthropic server.
4. Deploy through WP10 with Agent Observability enabled, verify one canonical record on all three surfaces, then verify privacy and cancellation.
5. Roll back by deploying the prior WP5/WP6 image/config; no protocol or persisted Gateway state migration is required.

## Open Questions

None for implementation. The target production Agent Observability endpoint, credentials, region, and application are deployment values supplied by WP10; WP8 defines and tests their strict configuration boundary without embedding environment-specific values.
