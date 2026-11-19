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

Declare `listModelCatalogProviderModels` in `kinds/manifest.cue` with `x-grafana-declared-authz-{resource,subresource,verb}` set to `modelcatalogs`, `available-models`, `get`; grant `ModelCatalog/available-models:get` only to `aigateway:catalogwriter` by default. The action reads inventory but spends provider request quota with a saved secret. Grafana checks the caller's access to the parent and the declared permission before decrypting or calling the plugin. Register the route with a fixed `catalog_available_models` label, reuse `parentCatalog` and `DecideCatalog`,. Those checks do not replace Grafana's declared check. A valid selector absent from the parent gets a fixed `404`; invalid stored configuration gets the existing fixed `424`; an admission refusal gets a fixed `429`. None echoes caller or provider text. Leave other grants and bindings unchanged.

### 3. Extract the listing once, keep the projections separate

Both consumers use `parseStackCatalog` and `resolveStackProvider`, then the same #836 listing traversal with its request building, headers, single-use probe transport and closed failure categories. Keep health's early stop once its targets are found, ordered fallback projection and existing response contract. The picker collects IDs until completion or a bound. Keep the current 3 s deadline per provider including body reads, 512 KiB decoded per response, three Anthropic pages of up to 100 requested entries and one OpenAI/compatible page. Keep health's run and fan-out bounds. Share health's admission gate across live health checks and picker requests: one active run per namespace and catalog name, four active runs per Gateway process, no queue. No redirects, transparent transport replay, or application retries.

On this base shared preflight refuses any `baseURL` override and any `openai-compatible` provider, with zero outbound requests. Once #778 lands, use the same normalized operator-approved destination as selection and health, not a picker allowlist or the raw spec. Append the fixed listing path without resetting the approved base path. The existing compatible listing path uses Bearer auth and one `GET models`; if an approved custom endpoint returns `404`, `405` or `501` for listing, return `notChecked` and leave manual entry available.

### 4. Return only the IDs the picker needs

```json
{ "status": "ok", "modelIds": ["claude-sonnet-4-6"] }
```

Use typed string statuses. On `200`, `ok` means the supported traversal completed without omissions imposed by the Gateway (and can include `[]`). `partial` means at least one page was read but the traversal could not finish (failure, bound or unsafe cursor), or IDs had to be dropped. Before any valid page, use health's `invalidKey`, `unreachable`, `refused`, or `notChecked`, with `[]`. Route and platform errors are distinct from provider results. Never treat missing IDs as invalid; even a successful listing can omit aliases or models the account can invoke.

Deduplicate exact IDs, keep at most 512 distinct IDs of at most 256 bytes each, and sort the kept IDs. Omit IDs with control characters or any decrypted key from the parent, including its HTTP header wire form and key text embedded in another ID. Never serialize upstream URLs, raw bodies, errors, cursors or other listing metadata. The shared traversal passes only a nonempty, nonrepeating Anthropic cursor of up to 1 KiB as an encoded query value on the same destination; apply the same key filter to cursors before sending them. Credentials go only in the provider auth header. Use fixed telemetry labels and classifications; do not attach inventory, selectors, credentials or request/response details to Gateway logs, metrics or traces.

### 5. Do not cache listings

Reuse a provider's pending request inside the editor if the list opens again before that request settles. Closing the list hides its result but does not abort the request. Forget pending requests after a successful save, including key replacement, and remove each request from the editor once it settles. The shared admission gate still bounds work across providers and health checks. Do not cache completed results. Gateway responses use `Cache-Control: no-store`.

### 6. Keep manual entry available throughout

Make the provider model ID field an editable combobox only on provider cards with a saved key and parent read, update and listing permissions. `CatalogPage` supplies the listing callback through `CatalogEditor`; `NewCatalog` uses the same form with a plain input and no callback. Opening the field with a click, text edit or arrow key fetches one listing while focus alone does not. Filter the results locally as the writer types. Click or Enter on a highlighted suggestion copies the exact ID into the field without saving; Tab and Escape close the list without changing the field. Only the existing **Add model** form submits the ID, whether typed or selected. No filtering on provider type as a substitute for backend policy.

Show fixed messages for empty inventory, no search matches, partial results and listing or route failure. Both listing and health explain how to retry when the catalog is busy (`429`). Reopening the list can retry; keep the typed ID intact and usable while loading or on any outcome. Render IDs as text, not markup or links. Drop result state on picker close, provider removal, any successful catalog write, navigation or loss of access; ignore responses from superseded requests. No persistence in the browser and no automatic refresh.

## Risks / Trade-offs

- First-time setup remains manual. Explain the limitation instead of adding a draft egress route or relying on a not-yet-supported blank catalog.
- Successful listing only describes what the provider returned. Keep manual IDs available and call out partial results; do not surface health or inference claims in the picker.
- Admission is per process, not a distributed rate limit. A writer can still retry a failing listing; no fan-out to other accounts, background polling, or automatic retry.

## Migration Plan

No data migration. Ship the regenerated manifests, route and UI together. Rollback removes the new action and suggestions; catalogs and manual entry still work. Document the saved-only flow and update the catalog guide, Gateway architecture and glossary where they enumerate these actions.
