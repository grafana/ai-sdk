# Proposal

## Why

A stack can store `ModelCatalog` resources and call them through their catalog routes. But the app has only an overview page, so administrators have to write catalogs as raw API objects, with keys in request bodies, and nothing in the app shows a catalog's base URL. The docs also don't say how a plugin offers Grafana models and lets a customer opt in to their own catalog.

## What Changes

- **A "Model catalogs" page** in the app, built from `@grafana/ui` and `@grafana/runtime`, on the `ModelCatalog` API of the request namespace:
  - a list of catalogs with their URLs, providers and models;
  - a page to create a catalog: name it, connect providers with their keys and add their models;
  - a page per catalog, organized by provider: each provider with its key and the models it serves, a model added by its provider model ID alone, and its details, such as aliases and fallbacks, in a drawer; each change is saved at once;
  - delete with a confirmation.
- **Controls follow the caller's permissions.** The page reads the caller's Grafana permissions for the `modelcatalogs` `get`, `create`, `update` and `delete` actions, not their org role. Without `get` it shows nothing. Without `create`, `update` or `delete` the matching control is absent or read-only. So a Viewer, or an Editor without the writer role, sees catalogs read-only.
- **Write-only keys.** A key typed into the page is sent only as `secure.<provider>.create`. It is never read back or displayed: a saved key shows as configured, with a control to replace it. An update omits every unchanged key, so Grafana keeps the stored value. It sends `remove` for a removed provider's key.
- **Platform errors as returned.** Admission refusals, authorization refusals, conflicts and name errors are shown with the message Grafana returns, and the dialog keeps its input. The page checks only missing values and what a request body can't express, such as a model ID the catalog already has. It leaves format and policy rules to admission.
- **Overview:** for callers who can read catalogs, the overview lists each catalog's base URL, `.../namespaces/{namespace}/modelcatalogs/{name}`, with a copy button.
- **Tests:**
  - Jest tests for the request bodies, the permission matrix, key states, error display and the overview;
  - Playwright tests under `ai-gateway/tests/` for the Admin flow, Viewer and Editor read-only pages, and the overview URLs.
- **Docs:**
  - the Gateway README: managing catalogs in the app, and when role and token changes take effect;
  - `ai-gateway/docs/model-catalog.md`: how stack catalogs relate to the catalog package;
  - the client guide: a "Grafana models and customer catalogs" section.
- **No backend change:** no kind, role, route, admission or manifest change.

## Capabilities

### New Capabilities

- `gateway-catalog-management`: managing stack model catalogs from the Gateway app. Covers the catalog page, permission-driven controls, write-only keys, platform error display and catalog base URLs on the overview.

### Modified Capabilities

- `gateway-model-catalog-kind`: operator documentation covers managing catalogs in the app, the roles, and when edits, removed roles and revoked tokens take effect.
- `grafana-gateway-client`: client guidance for offering Grafana models together with a customer's own catalog.

## Impact

- **Code:** `ai-gateway/src/`:
  - new catalog pages, dialog components, API module and permission helper;
  - `components/App/App.tsx` routes;
  - `pages/Overview.tsx`;
  - `constants.ts`;
  - `plugin.json` gains a page include.
  
  The generated kind types are used as they are.
- **APIs:** none. The page uses the existing `ModelCatalog` API with `getBackendSrv`.
- **Dependencies:** none added.
- **Tests:**
  - Jest specs next to the new components;
  - `ai-gateway/tests/`: a new `catalog-page.spec.ts`, an overview test, and a browser-user fixture in `fixtures.ts`.
- **Docs:**
  - `ai-gateway/README.md`, `ai-gateway/src/README.md` and `ai-gateway/docs/model-catalog.md`;
  - `docs/providers/grafana-gateway.md` and `docs/guides/gateway-authentication.md`.
- **Not parity-sensitive:** the Vercel AI SDK has no catalog UI. No stream, chunk, provider or wire format changes.
