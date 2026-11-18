# Design

## Context

`CatalogEditor` uses the same `ProviderCard` quick-add form in `CatalogPage` and `NewCatalog`. `ModelDrawer` edits an existing model's targets, app-facing ID and details; it is not the add flow. Draft keys live in `NewCatalog` until **Create**. In a saved catalog, each confirmed edit is a versioned update and the page continues from the returned object. Health runs only on the verified saved parent and currently retains matches to configured targets, not inventory.

## Goals / Non-Goals

- Make it easier to pick an exact ID from the right saved provider account; keep the current manual input and save semantics.
- Do not list from draft keys, call inference, verify capabilities, import a whole inventory into the catalog, or change `ModelDrawer` and its fallback suggestions.

## Decisions

### 1. Use the saved provider key (option a)

The create form stays manual-only, with a hint that browsing is available after save. The first catalog save needs at least one provider and model, so a writer who doesn't know the first ID must get it from the provider; we will not add a hidden placeholder or silently save a partial catalog. **Connect a provider** on the saved page already persists the new provider and key immediately. Browsing can become available there as soon as that save succeeds, before adding that provider's first model.

Option (b) would help on the first model, but it introduces a credential-bearing request and an additional egress path that doesn't have a saved parent. A separate proposal would need to threat-model request and telemetry redaction, destination policy, authorization for callers without a parent, and rate limits independent of catalog identity. Don't send a draft key directly to providers from the browser either. Saving an empty catalog first (option c) depends on a separate admission and UX change and is not a prerequisite for this picker. No listing on create, save, provider connection, page load, or in the background.

### 2. Add a catalog action with a declared permission

`POST /apis/aigateway.ext.grafana.app/v1alpha1/namespaces/{namespace}/modelcatalogs/{name}/available-models?provider={providerName}` accepts no body. A single required query parameter selects a saved provider by instance name, not type. Declare and bound the DNS-label selector (1–63 characters); reject missing, malformed, repeated or extra parameters and any nonempty body with a fixed `400`. The selector is only a map lookup in the validated parent, not part of the upstream URL. Use a single static subresource, as with health, rather than a namespace-level or variable provider subresource. POST avoids link prefetch and Grafana's GET-to-HEAD forwarding. Do no provider work for GET or HEAD.

Declare `listModelCatalogProviderModels` in `kinds/manifest.cue` with `x-grafana-declared-authz-{resource,subresource,verb}` set to `modelcatalogs`, `available-models`, `get`; grant `ModelCatalog/available-models:get` only to `aigateway:catalogwriter` by default. The action reads inventory but spends provider request quota with a saved secret. Grafana checks the caller's access to the parent and the declared permission before decrypting or calling the plugin. Register the route with a fixed `catalog_available_models` label, reuse `parentCatalog` and `DecideCatalog`, and carry UID and resource version from that verified parent. Those checks do not replace Grafana's declared check. A valid selector absent from the parent gets a fixed `404`; invalid stored configuration gets the existing fixed `424`; an admission refusal gets a fixed `429`. None echoes caller or provider text. Leave other grants and bindings unchanged.

### 3. Extract the listing once, keep the projections separate

Both consumers use `parseStackCatalog` and `resolveStackProvider`, then the same #836 listing traversal with its request building, headers, single-use probe transport and closed failure categories. Keep health's early stop once its targets are found, ordered fallback projection and existing response contract. The picker collects IDs until completion or a bound. Health does not read from or populate the inventory cache: **Check health** should still check the provider now. Keep the current 3 s deadline per provider including body reads, 512 KiB decoded per response, three Anthropic pages of up to 100 requested entries and one OpenAI/compatible page. Keep health's run and fan-out bounds. Share health's admission gate across live health checks and picker cache misses: one active run per namespace and catalog name, four active runs per Gateway process, no queue. Cache hits make no live probe. No redirects, transparent transport replay, or application retries.

On this base shared preflight refuses any `baseURL` override and any `openai-compatible` provider, with zero outbound requests. Once #778 lands, use the same normalized operator-approved destination as selection and health, not a picker allowlist or the raw spec. Append the fixed listing path without resetting the approved base path. Check policy before any cache lookup, so a now-disallowed endpoint cannot get a cached result. The existing compatible listing path uses Bearer auth and one `GET models`; if an approved custom endpoint returns `404`, `405` or `501` for listing, return `notChecked` and leave manual entry available.

### 4. Return only the IDs the picker needs

```json
{ "status": "ok", "modelIds": ["claude-sonnet-4-6"] }
```

Use typed string statuses. On `200`, `ok` means the supported traversal completed without omissions imposed by the Gateway (and can include `[]`). `partial` means at least one page was read but the traversal could not finish (failure, bound or unsafe cursor), or IDs had to be dropped. Before any valid page, use health's `invalidKey`, `unreachable`, `refused`, or `notChecked`, with `[]`. Route and platform errors are distinct from provider results. Never treat missing IDs as invalid; even a successful listing can omit aliases or models the account can invoke.

Deduplicate exact IDs, keep at most 512 distinct IDs of at most 256 bytes each, and sort the kept IDs. Omit IDs with control characters or a verbatim decrypted key, including key text embedded in another ID. Never serialize upstream URLs, raw bodies, errors, cursors or other listing metadata. The shared traversal passes only a nonempty, nonrepeating Anthropic cursor of up to 1 KiB as an encoded query value on the same destination; stop rather than forward a cursor containing the key. Credentials go only in the provider auth header. Use fixed telemetry labels and classifications; do not attach inventory, selectors, credentials or request/response details to Gateway logs, metrics or traces.

### 5. Cache successful inventory for 30 seconds

Use a bounded process-local cache (32 entries, evict expired entries before evicting the oldest). Key by namespace, catalog UID, resource version and provider instance name, all from the authorized parent except the resolved name. Authorize and validate on every request, including a hit. Do not cache if UID or resource version is missing. Cache only `ok` arrays, including empty arrays, for 30 s from completion; no renewal on reads. Partial and failed results should be immediately retryable. Store no spec, cursor, key or key-derived value. Use `Cache-Control: no-store` even on a cache hit. A saved edit, including key replacement, changes the resource version; delete/recreate changes the UID. Neither needs a cache invalidation callback. Verify key replacement against the pinned Grafana in CI.

No cache would be simpler but would repeat provider requests whenever writers reopen the picker. A cache keyed just by catalog or provider name could mix namespaces, accounts or saved keys. The short revision-scoped cache trades up to 30 s of upstream staleness for fewer requests without changing the health action.

### 6. Keep manual entry available throughout

Keep the existing provider model ID input and **Add model** button. On a provider card with a saved key, show **Browse models** only when `hasPermission` allows parent read, update and the listing action. `CatalogPage` supplies the saved-catalog listing capability through `CatalogEditor`; `NewCatalog` supplies no listing callback or key to the picker. While browsing, show progress and disable duplicate fetches. Search the returned IDs locally with a labelled, keyboard-operable control. Selecting an ID fills the existing field with its exact value; the existing validation and **Add model** confirmation still apply. No filtering on provider type as a substitute for backend policy.

Show fixed messages for empty inventory, no search matches, partial results and listing or route failure. Offer explicit retry after a failure or incomplete list; keep the typed ID intact and usable while loading or on any outcome. Render IDs as text, not markup or links. Drop result state on picker close, provider removal, any successful catalog write, navigation or loss of access; ignore responses from superseded requests. No persistence in the browser and no automatic refresh.

## Risks / Trade-offs

- First-time setup remains manual. Explain the limitation instead of adding a draft egress route or relying on a not-yet-supported blank catalog.
- Successful listing only describes what the provider returned. Keep manual IDs available and call out partial results; do not surface health or inference claims in the picker.
- Admission and TTL are per process, not a distributed rate limit. A writer can still retry a failing listing; no fan-out to other accounts, background polling, or automatic retry.

## Migration Plan

No data migration. Ship the regenerated manifests, route and UI together. Rollback removes the new action and suggestions; catalogs and manual entry still work. Document the saved-only flow and update the catalog guide, Gateway architecture and glossary where they enumerate these actions.
