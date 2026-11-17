# Design

## Context

- **Not parity-sensitive** (see proposal). The registered upstream baseline in `test/conformance/upstream.yaml` has no catalog UI to compare against. Platform references:
  - `@grafana/ui`, `@grafana/runtime` and `@grafana/data` 13.3.0 and `@grafana/plugin-e2e` 3.15.1, from `ai-gateway/package.json`;
  - Grafana behavior from the local source snapshot (`~/src/grafana`, exact version unknown) and the CI image. Anything version-dependent is re-checked in the Playwright run.
- **The app today:**
  - `App.tsx` routes every path to `Overview`;
  - `plugin.json` includes one page;
  - `Overview.tsx` derives the Gateway URL from `config.appUrl` and `config.namespace` (`gatewayEndpoint`);
  - there is no API client, form or permission code;
  - the generated kind types are in `src/generated/modelcatalog/v1alpha1`: `ModelCatalog`, `Spec`, `InlineSecureValue`.
- **The API:** `ModelCatalog` objects live at `/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/{ns}/modelcatalogs[/{name}]`.
  - Reads return `secure.<provider>.name` only.
  - Writes follow Grafana's inline secure value rules: `create` stores a secret, `remove` drops it, and an omitted key keeps the stored value.
  - Admission refuses an invalid catalog with `400` and a Kubernetes `Status` whose `message` names the field and never contains a key. RBAC refuses with `403`, a stale `resourceVersion` gives `409`, and a bad name gives `422`, each as a `Status`.
- **Permissions in the browser:** Grafana puts the user's permission map into boot data (`pkg/api/index.go`, `BuildPermissionsMap`), including actions from plugin roles bound to basic roles. `@grafana/runtime` exposes it as `hasPermission(action)`. The plugin roles register these actions (`pkg/router/plugin_roles.go`):
  - `aigateway.ext.grafana.app/modelcatalogs:get`, which covers get and list;
  - `:create`, `:update` and `:delete`.

  `tests/catalog.spec.ts` already shows Viewer and Editor hold only `:get`, and Admin holds all four.
- **End-to-end:**
  - CI runs `scripts/e2e.sh` against an OSS Grafana image with anonymous Admin;
  - plugin-e2e logs the browser in as `admin`;
  - `tests/fixtures.ts` gives API contexts for Viewer, Editor and Admin service accounts;
  - OSS Grafana can't assign plugin roles to users through custom role assignment.

## Goals / Non-Goals

**Goals:**

- A catalog admin never has to hand-write the API object, and a key typed into the page can't leak back into it.
- The page agrees with what Grafana would allow, because it reads the same permissions.

**Non-Goals:**

- Client-side copies of admission's checks, beyond what a request body can't express.
- Showing secret names, key history, or the catalog's discovery output.
- Calling catalog routes from the page, and pagination beyond one list call.
- Backend, manifest, role or kind changes.

## Decisions

### Routing and pages

| Path under `/a/grafana-aigateway-app/` | Page | Needs |
|---|---|---|
| `overview`, `/`, unknown | Overview | — |
| `catalogs` | Catalog list, with the new catalog dialog | `get`; `create` to create |
| `catalogs/:name` | Catalog page, read-only or editable | `get`; `update` to change, `delete` to delete |

- `plugin.json` adds a "Model catalogs" include at `/a/%PLUGIN_ID%/catalogs` with `addToNav`. It also sets `action: aigateway.ext.grafana.app/modelcatalogs:get`, so Grafana shows the nav entry only to readers. The page checks the permission anyway, because the include action gates navigation, not the URL.
- `ROUTES` gains `Catalogs`, and `catalogPageURL(name)` builds a catalog's path. `App.tsx` adds the two routes before the catch-all. The catalog page passes `pageNav` to `PluginPage` for breadcrumbs.
- Creating is a dialog on the list, not a route, so every name, `new` included, has its own `catalogs/:name` page.

### API module

`src/api/modelCatalogs.ts` wraps `getBackendSrv()`. It offers list, get, create, update and delete. Every request is sent with `showErrorAlert: false`, so the page owns error display and Grafana shows no toast as well.

- **Base path:** `${config.appSubUrl}/apis/${API_GROUP}/${API_VERSION}/namespaces/${config.namespace}/modelcatalogs`. Grafana core uses the same pattern in `getAPIBaseURL`.
- **Types:** the generated `ModelCatalog`, `Spec` and `InlineSecureValue`; no hand-written duplicates.
- **`errorMessage(err)`** returns `err.data.message` for a fetch error whose body is a `Status`, otherwise `err.data.message ?? err.statusText ?? err.message`. It doesn't rewrite the message.
- **Alternatives considered:**
  - RTK Query or the `@grafana/api-clients` packages: they aren't dependencies;
  - a generic Kubernetes client: one kind doesn't justify one.

### Permissions

`src/permissions.ts` exports the four action names and `catalogPermissions()`, which returns `{read, create, update, delete}` from `hasPermission`. It's a plain function, not a hook, because boot data doesn't change while the page is open.

- Every control asks this one helper. Nothing checks `orgRole`.
- Boot data is loaded when the page loads, from Grafana's permission cache, so a permission change shows on a later reload, once that cache refreshes. Grafana stays authoritative: a refused write shows its `403` like any other error.
- **Alternative considered:** probing with requests, such as a dry-run create. Rejected: it adds a write call for every view, and `hasPermission` is the documented plugin API that core features use.

### Page layout

The pages follow Grafana's admin settings patterns, the same ones the Assistant app's settings use: a page header with a one-line description and its actions, content limited to the `lg` breakpoint, resources as rows or cards with their state and actions, and create and edit in dialogs. Everything is built from `@grafana/ui`; the Assistant's own design system isn't a dependency.

- **List:** one `Card` per catalog. It shows the name, which links to the catalog's page, the provider and model counts, the base URL with a Copy URL button, a badge per provider type, and, with `delete`, a Delete action behind a `ConfirmModal`. With `create`, "New catalog" sits in the page header's actions; an empty namespace shows an `EmptyState` that explains catalogs and offers the same button.
- **Catalog page:** the header shows the name, the base URL with a copy control and, with `delete`, "Delete catalog". Below are two panels:
  - **Providers:** a row per provider with its name, a type badge, "Key saved" and the models that use it;
  - **Models:** a row per model with its ID, display name, description, its route ("Calls claude › claude-sonnet-4-6, then gpt › gpt-5") and aliases.

  With `update`, each panel has an Add button and each row Edit and Remove. A reader sees the same rows, every value included, and a read-only notice.

### Changes and key behavior

Each change is saved on its own, when the writer confirms it, rather than as a draft of the whole catalog:

- **Create** is one dialog with the least the platform accepts: the catalog name, with a live preview of its base URL; a provider with its type, name and key; and a first model with its ID, display name and provider model. The provider name follows the type, `anthropic` or `openai`, until the writer changes it. Create sends `apiVersion`, `kind`, `metadata.name`, `spec` and `secure`, then opens the catalog's page.
- **Provider dialog:** type, name and key. **Model dialog:** ID, display name, description, aliases, then routing: the primary provider and model and an ordered list of fallbacks, at most seven, with move and remove controls. A target's provider is a `Combobox` over the catalog's providers with their types, plus a stale name a target still references so admission can report it. Aliases use `TagsInput` with a 128-character limit, the documented alias limit, because its default of 50 would block valid aliases.
- **Saving** a dialog sends one update of the stored object: its `metadata` unchanged, including `resourceVersion`, so a concurrent change gives `409`; the whole new `spec`; and `secure` only when non-empty. The page then shows the object the update returned, so the next change carries its new resource version without a reload. A refused save keeps the dialog open with its input and the returned message.
- **Removing** a provider or model asks for confirmation, then updates the same way. A provider that a model calls can't be removed, and says so; the last model can't either, because the platform refuses a catalog without one.
- **Names are fixed once saved.** A catalog name is its URL. A provider name keys its secure value, and Grafana refuses reusing a secret under another key, so a rename would mean a new key anyway. A model ID is what applications request.
- **Round trips:** the dialogs edit the stored `Provider` and `Model` objects in place, so fields the page doesn't edit are sent back unchanged.

`src/components/Catalog/catalogSpec.ts` holds the pure parts: spec updates (`withProvider`, `withoutProvider`, `withModel`, `withoutModel`, `modelsUsing`), the secure values of a change, and the local checks. The secure values of a write:

| Change | Sent |
|---|---|
| Add a provider, or create a catalog | `create` for its key |
| Save a provider without a new key | nothing (kept) |
| Save a provider with a new key | `create` |
| Remove a provider | `remove: true` for its stored key |
| Any model change | nothing |

- **Key field:** `SecretInput` from `@grafana/ui`. It shows as configured when the provider has a stored key, no key has been typed and its type is unchanged. Reset opens the field for a replacement, which is then required. A type change on a stored provider also requires a new key: the stored key belongs to the old provider's account, and admission doesn't tie keys to types. The typed value is never written anywhere except the request body, and closing the dialog drops it.
- **Local checks**, shown on their fields: a missing value, a provider name or model ID the catalog already has, which the map-shaped body can't express, and a retyped stored provider without a new key. Name, ID and alias formats, alias collisions and policy limits go to admission; the field descriptions state the formats.
- **Alternatives considered:**
  - a single form of the whole catalog with one Save. It allows bulk edits, but a long form is hard to scan, unsaved changes can be lost, and a refusal shows far from the field that caused it;
  - a JSON spec editor (`CodeEditor`) with key fields per provider. Users would have to know the schema, keys and providers can drift apart in the editor, and Jest testing of Monaco is awkward.

### Errors

- Load errors are an `Alert` with severity `error` and the `errorMessage` text above the content. A refused dialog save shows the same inside the dialog, and a refused removal or delete above the content.
- In-app links use `react-router-dom`'s `Link`, or a `Card` `href`, which Grafana routes in the app, not `TextLink`. `@grafana/ui`'s internal `Link` reads Grafana's v5-compat router context, which is the same router at runtime but a different one in Jest.

### Overview

- A "Model catalogs" section after "Endpoint", rendered only when `read` is true. It lists catalogs once on mount.
- For each catalog it shows the name, linking to its page, and a `CodeSnippet` of `catalogBaseURL(endpoint, name)`, which is `${gatewayEndpoint(...)}/modelcatalogs/${encodeURIComponent(name)}`.
- One sentence says that Editors and Admins can call catalog URLs (`aigateway:modelinvoker`), with a link to the catalog page.
- With no catalogs, it shows a short line with the link. A load failure shows an inline `Alert` with severity `warning` and the returned message. The quick start is unaffected.

### Docs

What exists already:

- README "Store model catalogs in Grafana": the kind, keys, roles (`catalogreader`, `catalogwriter`, `modelinvoker`), the base URL, "an edit applies on the next call" and the one-minute decrypted-key cache.
- The client guide's "Authenticate the client" section covers the catalog base URL, the `403` and `404` responses, and links to the README.
- The glossary has the kind, catalog routes and `modelinvoker`.

What's added:

- **README:**
  - one paragraph on managing catalogs on the app's page, where the overview shows catalog URLs;
  - a bullet on propagation: edits apply on the next call, but a removed role or a revoked token can keep working for a while, because Grafana and Grafana Cloud cache permissions and tokens. No durations are given. At `v13.3.0` the legacy permission cache lasts 60 seconds (`pkg/services/accesscontrol/acimpl/service.go`), but the authz service's permission cache TTL is configurable (`pkg/services/authz/rbac/service.go`), and Cloud access tokens live until they expire, so no single number holds;
  - the Install step's overview sentence also names catalog URLs.
- **`ai-gateway/src/README.md`** (the plugin catalog listing): one sentence on the catalog page.
- **`ai-gateway/docs/model-catalog.md`:** a short "Stack catalogs" section near the top. It says this package builds catalogs inside a Go service, while stack catalogs are `ModelCatalog` resources the Gateway serves per stack, and links to the README section.
- **`docs/providers/grafana-gateway.md`:** a new "Offer Grafana models and customer catalogs" section after "Discover and select public models". It covers:
  - one client per route, since a client has one base URL;
  - storing the customer's choice (catalog name and model ID) in the consuming plugin's settings;
  - in Cloud, operator models need a backend in `access_internal_services` using token exchange;
  - calling the catalog route on behalf of the user with `WithOnBehalfOf`. Verified at `v13.3.0`: `pluginroute.checkDeclaredAccess` (`routes.go`) calls the access client, which is authlib's `authz.ClientImpl` (`pkg/services/authz/rbac.go`). At authlib `ed7cea0f2b27`, `Check` requires the token's delegated permissions to grant `modelcatalogs:get` (`CheckServicePermissions`), then checks the user, who needs the route grant from `aigateway:modelinvoker`;
  - why falling back to Grafana models is discouraged: the customer chose their provider and account, a silent fallback sends prompts elsewhere and hides a broken catalog, and the Gateway never crosses sources itself.

  Any Go snippet reuses calls already in the guide, so no new API is invented.
- **`docs/guides/gateway-authentication.md`:** the overview sentence names catalog URLs.
- **Architecture walkthrough and glossary:** they don't describe the frontend. Recheck them after implementation and change them only on drift.

### Tests

**Jest**, with `@grafana/runtime` mocked for `getBackendSrv`, `config` and `hasPermission`:

| Acceptance | Tests |
|---|---|
| Admin creates, changes, deletes; keys write-only | The `catalogSpec` tests: spec updates, the secure values of each change in the table above, local checks, and a stored model's round trip with fields the page doesn't edit. The API module sends `showErrorAlert: false` and builds URLs with `appSubUrl` and namespace. The create body carries `create` and opens the catalog's page. Each dialog sends one update of the loaded object: `create` for an added or replaced key, nothing for a kept one, `remove` for a removed provider, and fallbacks in their order. A provider in use and the last model can't be removed. The typed key isn't in the DOM after a save. Delete calls the API only after confirmation. |
| Viewer and Editor read-only | A permission matrix that sets permissions independently of `orgRole`: an Admin without `update` sees no change controls, an Editor with `update` does, and Delete follows `delete`. Without `get`, no request and no content. |
| Admission errors as returned | A refused create or update with a `Status` body shows its `message` verbatim in its dialog and keeps the input; a refused removal shows it on the page. A taken provider name and missing values block the request. |
| Overview base URLs | Lists catalog URLs, including `appSubUrl` and namespace encoding. No request without `get`. Empty state. A load error doesn't hide the endpoint. |
| Routing | `App` renders the list, a catalog's page, `catalogs/new` as a catalog named `new`, and the overview at their paths. |

**Playwright:**

- `tests/catalog-page.spec.ts` is new.
- `fixtures.ts` gains a worker-scoped `users` fixture. It creates a Viewer and an Editor login user through the admin API, deletes them afterwards, and offers `pageAs(role)`, a new browser context logged in through `POST /login`. Admin tests use the default admin page.

| Acceptance | Test |
|---|---|
| Admin create | Create a catalog in the dialog with a sentinel key. The page opens the catalog, whose provider shows "Key saved". The page, console and fetched responses never contain the sentinel, and an API `GET` shows only `secure.<p>.name`. |
| Admin edit, keys kept | Edit a model's display name: the `PUT` has no `secure` entry and the loaded resource version, and the API keeps the secure name. Then replace the key without reloading: the `PUT` carries only that provider's `create` and the resource version the first update returned. After a reload the key still shows as saved, and the sentinels appear nowhere. |
| Admin provider add and remove | Add a provider with a sentinel key: the `PUT` carries its `create`. Remove it: the `PUT` carries its `remove`, and the API keeps only the other key. |
| Admin delete | Delete from the list and confirm. The API returns `404` and the list drops it. |
| Admission errors | Create a catalog whose model ID has a space. Admission refuses it (`public ID`), the dialog shows the returned message verbatim and keeps its input, and the visible page, console and fetched responses have no sentinel. A provider name policy refusal was avoided because the name is also the secure key, which Grafana might check before admission. |
| Concurrent edit | Open the catalog, update it through the API, then save a model. The dialog shows the `409` message, and the other change stays. |
| Viewer read-only | `pageAs('Viewer')`: the list shows the catalog, with no New or Delete. The catalog page shows the saved key and a read-only notice, with no add, edit or delete controls. |
| Editor read-only | The same as `pageAs('Editor')`. |
| Overview | Create a catalog through the API. The overview shows `.../namespaces/default/modelcatalogs/<name>`. |

- "Base the UI on permissions, not roles" can't be shown end to end on OSS, which can't grant `catalogwriter` to an Editor. The Jest permission matrix carries it, and the Viewer and Editor runs prove the bound roles.
- The existing `e2e.sh` sentinel log check also covers these keys, because the spec uses the same sentinel prefix.
- After each save, the spec also checks that the typed sentinel appears in no browser console message and no response body the page fetched, recorded through Playwright's `console` and `response` events. Watch requests are skipped, because their bodies never finish.

## Risks / Trade-offs

- [Boot-data permissions are loaded once per page load] → a role change shows on a later reload, once Grafana's permission cache refreshes. The server refuses stale writes with `403`, and the page shows the error.
- [Include `action` support or plugin validator warnings] → if the validator or Grafana 13.3 rejects the include action, fall back to the default Viewer include and keep the in-page check.
- [A future spec field the page doesn't edit] → the dialogs edit the stored objects in place, so such a field round-trips unchanged; a Jest round-trip test covers it. It can't be edited until a dialog gains its input.
- [One write per change] → each write is small and admission checks it at once, but a change across many models takes several confirmations.
- [Grafana list limits] → the page makes one list call. A stack is expected to hold few catalogs; paginate if the platform starts truncating.
- [Overview in Grafana Cloud], where catalogs with keys are refused → the list is empty or fails. Either case renders inline without affecting the quick start.
- [Browser login users in e2e depend on the login form] → the compose base keeps the login form enabled. plugin-e2e already logs in `admin` the same way.

## Migration Plan

Additive frontend change. Rolling back removes the page and the overview section; stored catalogs are unaffected.

## Follow-ups

- **#760** allows `baseURL`, `openai-compatible`, `providerName` and `supportsStructuredOutputs` in stack catalogs. Once both land, the provider dialog needs:
  - a third type option;
  - inputs for those three fields, shown by type;
  - the matching Jest and Playwright cases.

  The dialog already edits the whole stored `Provider` object, so the spec updates and the secure values don't change.
- **Provider-level settings,** such as serving every model of a provider without listing each one, belong in the provider dialog and in a summary on the provider's row. The models panel keeps listing the models a catalog names.

## Decisions recorded at review

- A structured form; the nav include gated on `modelcatalogs:get`; a `pageAs(role)` fixture; base URLs on the overview only.
- Redesigned after a UI review: list cards and a catalog page with provider and model panels, after Grafana's admin settings patterns; create, edit and remove in dialogs, each saved at once; names fixed once saved; create as a dialog, so catalogs live at `catalogs/:name`.
- Cache durations only if verified at `v13.3.0`, otherwise none (none given, see Docs). On-behalf-of requirements only as verified in Grafana source (see Docs).
