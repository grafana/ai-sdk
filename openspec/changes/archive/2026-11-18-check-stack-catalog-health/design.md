# Design

## Context

On the catalog page branch (`fa6a1271`), catalog routes already receive a checked platform parent, and `parseStackCatalog` calls `stackcatalog.Parse` with the names of decrypted keys that have values. This checks the spec, provider policy and keys before selection; it exposes only a fixed catalog configuration error on failure. `StackCatalogLister` reuses it but only lists stored entries, so `config` cannot attest to upstream availability.

At the registered Vercel baseline (`@ai-sdk/gateway` 4.0.146, commit `0e8e0543`), `getAvailableModels()` reads Gateway `/config` metadata; it does not check credentials on a Grafana `ModelCatalog`. The AI Gateway row of `test/conformance/PARITY.md` already points to Gateway specs and tests. No registered provider adapter, ProviderWire projection, W3C transport context, SSE, or React hook behavior changes for this separate admin route.

## Goals / Non-Goals

- Confirm what the saved key and endpoint can list, and report every configured provider and target without leaking upstream data.
- Preserve the existing save path and the model invocation data plane. Do not verify inference, capabilities or fallback behavior by running a model.

## Decisions

### 1. Use the model-listing API, not a minimal generation

List once per named provider instance, including providers with no targets. Match only the configured backend IDs, exactly, across all primary and fallback targets; deduplicate within a provider, not across accounts. A minimal generation call per target would spend tokens, consume inference quotas, require provider- and modality-specific inputs, and still not establish full usability. Listing consumes API requests and may encounter endpoint permissions and rate limits, but creates no inference usage. No automatic retries or generate fallback.

| Type | Listing | What a missing target means |
| --- | --- | --- |
| Anthropic | `GET v1/models` relative to the validated base (native `https://api.anthropic.com`); use `x-api-key` and the same `anthropic-version` as the pinned native client. Read `data[].id`; when `has_more` is true, use `last_id` as the `after_id` query value. Stop on missing or repeated cursors, all targets found, or a bound. | `unknownModel` only after the complete native list. A missing alias may still be accepted by inference. |
| OpenAI | `GET models` relative to the validated base (native `https://api.openai.com/v1`); Bearer auth. Read `data[].id` from the single native list response. | `unknownModel` only after a complete native list. |
| `openai-compatible` (once allowed) | `GET models` under the approved API base (often ending in `/v1`); Bearer auth, one request, no assumed pagination. | Report exact matches only. An absent ID, or a listing API the backend does not implement, is `notChecked`, not a failed inference claim. |

Apply the conservative missing-model rule to **all non-native destinations**, even when configured as `anthropic` or `openai`. `ok` means a listing was accepted or a target was listed; some compatible endpoints accept lists without validating auth, and an endpoint may accept listing but deny inference. Do not consume provider-supplied continuation URLs. Only the Anthropic cursor is used, as an encoded query value on the same operation and destination.

### 2. Add a writer-gated catalog action

Use `POST /apis/aigateway.ext.grafana.app/v1alpha1/namespaces/{namespace}/modelcatalogs/{name}/health` with no payload or query. Although it does not write the catalog, it spends provider request quota. POST makes it an explicit action; a GET link is liable to prefetch, and Grafana's pluginroute can forward `HEAD` as `GET` when only a GET operation is declared. No namespace-level health route, and no Gateway work for GET or HEAD on this path.

Add `checkModelCatalogHealth` to `kinds/manifest.cue`, with `x-grafana-declared-authz-{resource,subresource,verb}` set to `modelcatalogs`, `health`, `get`. Regenerate the frontend and Go manifests and register the same method and path in `gateway/routes.go` with a fixed `catalog_health` route label. Add `{kind: "ModelCatalog/health", verbs: ["get"]}` to `aigateway:catalogwriter`; leave `catalogreader`, `modelinvoker` and role bindings unchanged. `get` is the authorization verb for the subresource, not a grant of base-resource reads or writes. The writer grant is the narrower default for actively using saved secrets and spending quota. Existing explicit role assignments remain supported; do not branch on org role.

Grafana reads and authorizes the parent and checks the declared action before decrypting keys and calling the plugin. In the Gateway, reuse `parentCatalog` and `DecideCatalog`: missing or mismatched route metadata or JSON identity yields a fixed internal error before access or HTTP-body handling; the existing caller check rejects invalid identity/namespace. `DecideCatalog` does not enforce the platform's specific health grant; the manifest does.

### 3. Check the stored revision only and share selection's validation

Run after save. A draft endpoint would take credentials and configuration in the request and open an additional egress path; save-time checks would tie catalog writes to external latency and availability. Neither is needed for a first useful diagnosis. Use the same `parseStackCatalog` preflight and resolved stack-provider construction as `SelectStackCatalog`, without constructing `LanguageModel`s or invoking middleware. A failed preflight (including a missing decrypted key) produces a fixed `424` with no probes; do not partially inspect unchecked entries or substitute operator/ambient keys.

On this base `stackcatalog.Parse` refuses any `baseURL` override and `openai-compatible`; health must do the same. When #778 lands, thread the same operator `outbound.CustomEndpoints` instance through the common preflight, which checks and normalizes destinations on every selection. Consume that validated provider configuration (including the normalized base URL) and shared default endpoint functions, not the raw spec or a second allowlist. Append only the fixed API-relative listing path without resetting the base path. The outbound redirect refusal applies equally to native and allowlisted destinations. A no-longer-approved endpoint fails the shared preflight for the entire catalog with `424` and zero traffic.

### 4. Report closed outcomes, separate from route errors

An accepted check returns `200` with `Cache-Control: no-store` even when checks fail or are incomplete. Keep all keys of the result maps local to the validated catalog; do not echo provider model inventory, cursors, or backend IDs. `fallback` contains one result per stored target in order, or `[]`. There are no separate rows for catalog aliases.

```json
{
  "providers": { "anthropic": "ok", "openai": "invalidKey" },
  "models": {
    "assistant": { "primary": "ok", "fallback": ["notChecked"] },
    "summarizer": { "primary": "unknownModel", "fallback": [] }
  }
}
```

Use typed string statuses. Before a valid page, upstream `401` is `invalidKey`; `403`, `429`, redirects, other unexpected HTTP responses and malformed success JSON are `refused`; timeouts, transport failures and `5xx` are `unreachable` (the custom endpoint's `404`/`405`/`501` no-listing case is `notChecked`). The model statuses for any of those provider failures are `notChecked`. A provider not started within the run budget, or whose first reply exceeds the byte bound, is `notChecked`. A valid page is enough for provider `ok`: if later pages fail or are capped, preserve observed matches and leave unconfirmed entries `notChecked`. Native `unknownModel` means not listed, not proof the identifier is invalid. `400` (unexpected input), host errors, and `429` (Gateway admission) are fixed route-level errors, not probe outcomes. Do not forward provider status codes, headers or bodies to the browser.

### 5. Bound the work and keep secrets out of telemetry

Use the model transport's proxy and dial policy, but issue a single round trip per page so redirects are classified without being parsed or followed. Use fresh HTTP/1 connections for listings so Go cannot transparently replay a failed idempotent GET on a reused connection or an HTTP/2 stream. Apply health-only request deadlines and a hard limit on decoded response bytes regardless of media type. Parse only bounded successful listing JSON (`data[]` with nonempty string IDs and the needed cursor fields). Close bodies on every exit; ignore unsuccessful response bodies and discard transport/decoder errors after classification. The provider key goes only in the relevant auth header. Log/measure with fixed route labels and enum categories only; don't call inference observation, attach request or response details to spans, or persist any keys or results.

Proposed fixed defaults: 8 providers per check (stable sorted names, mark excess as `notChecked`), at most 4 providers working at a time, 3 Anthropic pages of up to 100 requested entries, one OpenAI/compatible page, and 512 KiB decoded per response. This caps at 24 listing requests per check. Give each started provider 3 s including pagination and body reads, within an 8 s run deadline derived from the request. Stop promptly on cancellation. A failing provider does not stop others.

Admit one in-flight run per `(namespace, catalog name)` and at most four active runs per process. Never queue a run; reject with fixed `429`. Hold no state beyond the active runs. Writer-only access and fixed fan-out bound the work without a cooldown, token bucket, distributed coordination or new settings.

### 6. Annotate the saved page, not the editor flow

In `CatalogPage`, reuse the namespaced API helper and gate the button on `modelcatalogs:get` and `modelcatalogs/health:get` through `hasPermission`. Disable it while pending. Show narrow status labels on provider cards and model rows; show fallback labels in existing details. Keep **Key saved** independent of health; prefer labels like **Listing accepted**, **Model listed** and **Not listed** to a generic healthy badge. Use fixed messages for failures including busy/retry, configuration and permission, not raw response text. Clear results on successful writes or navigation and ignore any response from a check started before either. The response does not carry a resource version. No action or badges on the draft, list or overview; no automatic check.

### 7. Verify each boundary with deterministic tests

- **Unit:** white-box, table-driven Go tests with testify and a fake transport/server. Exercise provider path/auth (including an allowed base-path prefix once #778 lands), every result class, consumed vs incomplete pages, exact matches and ordered fallbacks, unused providers, response truncation, context cancellation, concurrency and admission, redirect refusal and secret sentinels. Use deterministic budgets/clocks for limit tests. Jest covers the health-only permission, no automatic requests, fixed labels/errors, and pending and stale-result states.
- **Gateway harness:** use `test/platformhost` and the real route table with a fake provider. Check the correct decrypted key and no provider traffic on absent/mismatched parent, invalid/missing keys, invalid caller, unsupported method, invalid input, rate refusal or disallowed endpoint. The harness does not enforce Grafana's declared roles; assert the manifest grant separately and prove actual authorization in Playwright.
- **Playwright is feasible on the base:** `docker-compose.e2e.yaml` already routes Grafana's native Anthropic traffic through the TLS `native-proxy.mjs` to `fake-anthropic.mjs`. Extend the fake with `GET /v1/models` and deterministic accepted and rejected key cases (distinct from existing generation tests). Use the real saved page and persisted keys, not a browser mock. Assert an Admin gets listed/unlisted models and key rejection, no key/upstream error in browser results, and Editor/Viewer lack the button and get `403` on direct requests to an existing catalog. CI supplies end-to-end evidence; no local Docker or real provider keys are required.

## Risks / Trade-offs

- Listing cannot prove inference or reliably resolve unlisted aliases. Say exactly what was checked and preserve `notChecked` for inconclusive probes.
- Very large catalogs can have unprobed providers on every run. Report the omissions rather than block admission; do not quietly turn the button into sustained polling.
- Per-process budgets do not coordinate replicas; the default role grant and fixed fan-out still apply at each replica.

## Migration Plan

No storage migration. Ship the generated manifest, backend and frontend together. Rollback removes the action and UI; existing catalogs and invocation keep working. Update the Gateway architecture and glossary, which currently describe only two catalog routes, and the catalog guide.

## Open Questions

- Do real catalogs commonly exceed eight providers or need more listing pages? Keep the conservative defaults and use operational feedback to decide whether a later change needs per-provider checks or larger budgets.
- Are provider-specific unlisted aliases common enough to justify a bounded model-record lookup? Defer rather than make a paid inference call or guess at alias equivalence.
- Does the pinned Grafana image expose the declared subresource action through `hasPermission` as expected? Assert it in CI; the manifest check remains authoritative regardless of the UI.
