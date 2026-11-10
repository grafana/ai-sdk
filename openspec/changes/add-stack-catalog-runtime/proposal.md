# Proposal

## Why

A stored `ModelCatalog` has no runtime effect. The Gateway builds models only from the operator catalog at startup, with keys from the plugin environment, and requires that catalog to start. Serving stack catalogs needs a runtime that builds a catalog's models per request from its spec and decrypted keys. That runtime must redact those keys and keep telemetry bounded across any number of catalogs. The catalog routes that use it come next.

## What Changes

- **Optional operator catalog.** The Gateway starts without `model_catalog`. The namespace routes then list no configured models and refuse every model ID as not found. BYOK is unchanged. An oversized or invalid catalog still fails startup.
- **Stack-catalog selection.** A new selection source, `stack-catalog`, built from a catalog spec, its decrypted secure values keyed by provider name, and the catalog's name:
  - the stored spec is checked on every request with the stack-catalog policy, shape and key checks;
  - only the provider candidates that the call's composed chain can attempt are constructed: the requested model's, plus those of any `gateway.models` entries, after `only` and `order`;
  - routing controls and fallbacks behave as for the operator catalog, including the 8-attempt bound;
  - `gateway.byok` is refused as an unsupported gateway control. There is no fallback to the operator catalog, server-held keys or ambient credentials;
  - candidates use the native Anthropic and OpenAI endpoints and the shared model HTTP client.
- **Request-scoped summary redaction.** A stack-catalog selection carries its catalog's decrypted keys. Native failure summaries in execution overviews and event-local errors redact them along with the startup keys. BYOK redaction is unchanged.
- **New `catalog configuration` failure category.** It covers a stored catalog that fails the checks, or a provider without a decrypted key. The fixed ProviderWire document is HTTP 424 `failed_dependency` with a fixed message, not retryable. It never echoes catalog content, check messages or keys. Both clients already accept this status, type and code.
- **A third observer chain for stack catalogs.** Its middleware is constructed once at startup and applied per call by source:
  - logs and Agent Observability use provider `grafana` and the catalog's canonical model ID;
  - model metrics use a fixed model label, `catalog`, through the same single metrics registration;
  - the catalog name joins the request observation, as log field `catalog` and Agent Observability metadata `gateway.catalog`. It is never a metric label;
  - operator Agent Observability records these calls metadata-only through the existing process-wide client.
- **Resource bounds.** Stack catalogs use the operator catalog's shape bounds. Per-request work is bounded by the stored spec and by the 8-attempt bound on constructed candidates, and nothing is cached across requests.
- **Docs.** README: `model_catalog` is optional, and stack-catalog limits are added to Limits. The architecture walkthrough and glossary cover the new source.
- No routes, manifest, account type or frontend change. A stored catalog still affects no route until the catalog routes land.

## Capabilities

### New Capabilities

- `gateway-stack-catalog-runtime`: building and selecting a stack catalog's models per request. Covers inputs, per-request validation, needed-only construction, routing and fallback, refused BYOK, the `stack-catalog` source and the execution overview.

### Modified Capabilities

- `gateway-plugin-host`: `model_catalog` becomes optional in host-compatible configuration.
- `gateway-provider-configuration`: provider keys can also come from a stack catalog's decrypted secure values, isolated from operator providers; behavior without an operator catalog.
- `gateway-text-observability`: a third chain for the `stack-catalog` source, the fixed `catalog` metric label, the catalog name in the trusted observation view, and per-request stack-catalog identity.
- `gateway-request-routing`: the source list at the selection seam gains `stack-catalog`.
- `gateway-execution-runtime`: the closed failure classification gains `catalog configuration`.
- `providerwire-v4-unary-runtime`: the fixed `catalog configuration` error document.
- `gateway-attempt-failure-evidence`: summary protection for request-scoped catalog keys.
- `gateway-resource-policy`: stack-catalog bounds, the attempt bound for stack-catalog chains, and the Limits ledger.

## Impact

- **Code:**
  - `ai-gateway/internal/execution`: `SourceStackCatalog`, catalog name and request keys on `Selection`, per-call summary redactor, the new sentinel and category;
  - `ai-gateway/providerwire/v4`: the fixed document;
  - `ai-gateway/pkg/internal/service`: stack-catalog selection, shared candidate construction and routed composition, the third observer chain, log policy by source, the observation catalog field;
  - `ai-gateway/pkg/internal/config` and `pkg/internal/gateway`: optional `model_catalog`, empty operator catalog.
- **APIs:** no change to ProviderWire requests or the discovery document. One new error document, using an already registered status, type and code. The new Go selection API is internal, for the catalog routes.
- **Tests:** unit tests for selection, construction counts, redaction, classification and documents. Observer composition tests: identity, bounded series across many catalogs, no keys in any sink. Gateway startup without `model_catalog`. Go and TypeScript client classification of the new document.
- **Docs:** `ai-gateway/README.md` (settings, Limits), `docs/concepts/gateway-architecture.md`, `docs/concepts/gateway-glossary.md`.
- **Not parity-sensitive:** `test/conformance/PARITY.md` lists the AI Gateway as a Go extension governed by `gateway-*` specs. ProviderWire requests, stream parts, UI chunks and provider conversion are unchanged, and no conformance fixture changes.
