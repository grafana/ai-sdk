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
| `catalogs` | Catalog list | `get` |
| `catalogs/new` | New catalog | `create` |
| `catalogs/edit/:name` | Catalog page, read-only or editable | `get`; `update` to change, `delete` to delete |

- `plugin.json` adds a "Model catalogs" include at `/a/%PLUGIN_ID%/catalogs` with `addToNav`. It also sets `action: aigateway.ext.grafana.app/modelcatalogs:get`, so Grafana shows the nav entry only to readers. The page checks the permission anyway, because the include action gates navigation, not the URL.
- `ROUTES` gains `Catalogs`, `NewCatalog` and `EditCatalog`, and `catalogPageURL(name)` builds a catalog's path. `App.tsx` adds the three routes before the catch-all. Sub-pages pass `pageNav` to `PluginPage` for breadcrumbs.
- `edit/:name` follows Grafana's data source pages (`/datasources/new`, `/datasources/edit/:uid`), which also serve read-only users. `catalogs/:name` next to `catalogs/new` was rejected: `new` is a valid catalog name and would be unreachable.

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

A catalog has three layers: the catalog, its providers, and the models they serve. A writer thinks "connect my Anthropic key, then choose the models to offer", so the catalog page is organized by provider, and the page derives what a common catalog repeats. The layout follows Grafana's admin settings patterns, the ones the Assistant app's settings use too: a page header with its actions, content limited to the `lg` breakpoint, and resources shown with their state and actions. Everything is built from `@grafana/ui`; the Assistant's own design system isn't a dependency.

- **List:** one `Card` per catalog. It shows the name, which links to the catalog's page, the provider and model counts, the base URL with a Copy URL button, and a badge per provider type. Catalogs are deleted only from their own page, so the list has no Delete action. With `create`, "New catalog" sits in the page header's actions; an empty namespace shows an `EmptyState` that explains catalogs and offers the same link.
- **Catalog page:** the header shows the name, the base URL with a copy control and, with `delete`, "Delete catalog". Below it is one card per provider:
  - the header shows the provider's type, its name when that differs from the type, its key state, Replace key and Remove;
  - the body lists the models whose primary is that provider: the model ID, the display name and provider model when they differ from it, its fallbacks ("Falls back to gpt-5 on openai") and its aliases;
  - the footer adds a model by its provider model ID, in one field.

  "Connect a provider" follows the cards. A model's ID opens its drawer.
- **Model drawer:** a `Drawer` with three sections: what the model calls (its primary provider and provider model), what applications see (model ID, display name, description, aliases), and its fallbacks, in order, with move and remove controls. The catalog's other models' primaries are offered as one-click fallbacks, and "Another provider model" adds a free row. A target's provider is a `Combobox` over the catalog's providers with their types, plus a stale name a target still references so admission can report it. Aliases use `TagsInput` with a 128-character limit, the documented alias limit, because its default of 50 would block valid aliases.
- **New catalog page:** three numbered steps on one page: name the catalog, with a live preview of its base URL; connect providers and add models, in the same provider cards; and create. Nothing is written until Create, because the platform accepts a catalog only with a provider and a model. Entered keys stay in memory, shown as "Key entered", and are sent once, in the create.
- A reader sees the same cards and drawer, without actions or a save button, and a read-only notice.
- Provider-level settings, such as serving every model of a provider without listing each one, belong on the provider card, next to its list of models.

### Defaults

| Field | Default | Asked for |
|---|---|---|
| Provider name | The type, such as `anthropic` | Only when the catalog already has a provider of that name; the suggestion is the type with the first free number |
| Model ID | The provider model ID | In the drawer, to serve the model under another ID |
| Display name | The model ID | In the drawer; left empty, it stays the ID |
| Primary | The card's provider and the entered provider model | In the drawer, which can move the model to another provider |

Admission still receives every required field, because the page fills these in. A provider's type is fixed once it's connected, because its key belongs to that type's account: another type is another provider. Provider and catalog names are fixed too: a catalog's name is its URL, and a provider name keys its secure value, which Grafana won't move to another key. A model's ID can change in the drawer, which warns that applications using the old ID break; the update carries the model under the new ID only.

### Changes and key behavior

On a stored catalog, each change is saved when the writer confirms it, rather than as a draft of the whole catalog. It is one update of the stored object: its `metadata` unchanged, including `resourceVersion`, so a concurrent change gives `409`; the whole new `spec`; and `secure` only when non-empty. The page then shows the object the update returned, so the next change carries its new resource version without a reload. A refused change keeps its dialog or drawer open with its input and the returned message.

- **Removing** a provider or model asks for confirmation. Removing a provider also removes the models whose primary it is, and the confirmation names them. A provider can't be removed while another provider's model falls back to it, and says which. A stored catalog's last model can't be removed, because the platform refuses a catalog without one.
- **Round trips:** the drawer edits the stored `Model` object in place, and providers are only added, removed or given a new key, so fields the page doesn't edit are sent back unchanged.

`src/components/Catalog/catalogSpec.ts` holds the pure parts: the spec updates, the defaults, the secure values of a change, the new catalog's entered keys and the local checks. The secure values of a write:

| Change | Sent |
|---|---|
| Connect a provider, or create a catalog | `create` for each key |
| Replace a key | `create` for that key |
| Remove a provider | `remove: true` for its stored key |
| Any model change | nothing |

- **Key fields** are empty `SecretInput`s, in the connect and Replace key dialogs only. A saved key is never in a field; the card shows that it's saved. The typed value is written nowhere except the request body, and closing the dialog drops it.
- **Local checks**, shown on their fields: a missing value; a provider name or model ID the catalog already has, which the map-shaped body can't express; and formats, so a writer learns of a bad value as they type rather than from admission: the catalog name follows Grafana's resource name rules (letters, digits and `-_.:`, at most 253 characters, not a reserved name such as `trash`), provider names are DNS labels because they key the secure values, and model IDs and aliases follow the Gateway's public ID format. The new catalog page checks the name as it is typed, and shows the catalog's URL, with a line on what applications use it for, only once the name is valid. Other fields follow one timing, from `useFieldErrors`: a format error shows once the writer leaves the field, a missing value only on save, and every shown error is re-checked on each change so it clears as soon as the value is fixed. Checking on every keystroke was rejected because a value being typed is often briefly invalid, such as a provider name ending in `-`; checking only on save, Grafana's default, leaves a bad value unreported until the end. Alias and ID collisions and policy limits go to admission.
- **Alternatives considered:**
  - providers and models in separate panels, cross-referenced by name. Every model then asks which provider it calls, and a writer keeps translating between the panels;
  - a models table with providers as a strip of key cards. It scales to many models with cross-provider fallbacks, but keeps the provider choice on every model and has no place for provider-level settings;
  - a single form of the whole catalog with one Save. It allows bulk edits, but a long form is hard to scan, unsaved changes can be lost, and a refusal shows far from its cause;
  - a JSON spec editor (`CodeEditor`) with key fields per provider. Users would have to know the schema, keys and providers can drift apart in the editor, and Jest testing of Monaco is awkward.

### Errors

- Load errors are an `Alert` with severity `error` and the `errorMessage` text above the content. A refused change shows the same inside its dialog or drawer; a refused quick add, under its field; a refused removal or delete, above the content.
- In-app links use `react-router-dom`'s `Link`, `LinkButton` or a `Card` `href`, which Grafana routes in the app, not `TextLink`. `@grafana/ui`'s internal `Link` reads Grafana's v5-compat router context, which is the same router at runtime but a different one in Jest.

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
| Admin creates, changes, deletes; keys write-only | The `catalogSpec` tests: spec updates, the defaults, the secure values in the table above, the new catalog's entered keys, local checks, and a stored model's round trip with fields the page doesn't edit. The API module sends `showErrorAlert: false` and builds URLs with `appSubUrl` and namespace. The new catalog page sends one create with every entered key, none of a removed provider, and opens the catalog. Each change on the catalog page sends one update of the loaded object: a quick-added model under its own ID, `create` for a connected provider or replaced key, `remove` and the removal of its models for a removed provider, a renamed model under its new ID only, and a one-click fallback. A provider used as a fallback and the last model can't be removed. Typed keys aren't in the DOM. Delete calls the API only after confirmation. |
| Viewer and Editor read-only | A permission matrix that sets permissions independently of `orgRole`: an Admin without `update` sees no change controls, an Editor with `update` does, and Delete follows `delete`. A reader's drawer is read-only. Without `get`, no request and no content. |
| Admission errors as returned | A refused create or update with a `Status` body shows its `message` verbatim where the change was made and keeps the input; a refused removal shows it on the page. A taken model ID and missing values block the request. |
| Overview base URLs | Lists catalog URLs, including `appSubUrl` and namespace encoding. No request without `get`. Empty state. A load error doesn't hide the endpoint. |
| Routing | `App` renders the list, the new catalog page, a catalog's page and the overview at their paths. |

**Playwright:**

- `tests/catalog-page.spec.ts` is new.
- `fixtures.ts` gains a worker-scoped `users` fixture. It creates a Viewer and an Editor login user through the admin API, deletes them afterwards, and offers `pageAs(role)`, a new browser context logged in through `POST /login`. Admin tests use the default admin page.

| Acceptance | Test |
|---|---|
| Admin create | On the new catalog page, name a catalog, connect a provider with a sentinel key and add a model, then create. The create carries the key, and the page opens the catalog, whose provider shows "Key saved". The page, console and fetched responses never contain the sentinel, and an API `GET` shows only `secure.<p>.name`. |
| Admin edit, keys kept | Edit a model's display name in its drawer: the `PUT` has no `secure` entry and the loaded resource version, and the API keeps the secure name. Add a model, then replace the key, without reloading: each `PUT` carries the resource version the previous update returned, and the key's carries only that provider's `create`. After a reload the key still shows as saved, and the sentinels appear nowhere. |
| Admin provider connect and remove | Connect an OpenAI provider with a sentinel key: the `PUT` carries its `create`. Remove it: the `PUT` carries its `remove`, and the API keeps only the other key. |
| Admin delete | The list has no Delete action. Open the catalog, choose Delete catalog and confirm: the page returns to the list, which drops it, and the API returns `404`. |
| Admission errors | Create a catalog with a model whose alias is its own ID. Admission refuses it (`alias collides`), the page shows the returned message verbatim and keeps its input, and the visible page, console and fetched responses have no sentinel. A provider name policy refusal was avoided because the name is also the secure key, which Grafana might check before admission. |
| Concurrent edit | Open the catalog, update it through the API, then save a model in its drawer. The drawer shows the `409` message, and the other change stays. |
| Viewer read-only | `pageAs('Viewer')`: the list shows the catalog, with no New catalog. The catalog page shows the saved key and a read-only notice, with no connect, key, or delete controls. `catalogs/new` says they can't create catalogs. |
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
- [Derived defaults hide a field the writer might want] → every default is shown and editable in the drawer, and a model added under its own ID can change ID later.
- [Grafana list limits] → the page makes one list call. A stack is expected to hold few catalogs; paginate if the platform starts truncating.
- [Overview in Grafana Cloud], where catalogs with keys are refused → the list is empty or fails. Either case renders inline without affecting the quick start.
- [Browser login users in e2e depend on the login form] → the compose base keeps the login form enabled. plugin-e2e already logs in `admin` the same way.

## Migration Plan

Additive frontend change. Rolling back removes the page and the overview section; stored catalogs are unaffected.

## Follow-ups

- **#760** allows `baseURL`, `openai-compatible`, `providerName` and `supportsStructuredOutputs` in stack catalogs. Once both land, the connect dialog needs:
  - a third type option;
  - inputs for those three fields, shown by type;
  - the matching Jest and Playwright cases.

  The connect dialog writes the whole `Provider` object, so the spec updates and the secure values don't change.
- **Provider-level settings,** such as serving every model of a provider without listing each one, belong on the provider card, next to its list of models.
- **Key and model checks:** the page can't tell whether a key works or a provider model exists, because keys are write-only and the Gateway has no endpoint that calls the provider with a stored key. Such an endpoint would let the page show a key's state, list the provider's models for the quick add, and refuse unknown ones.

## Decisions recorded at review

- A structured form; the nav include gated on `modelcatalogs:get`; a `pageAs(role)` fixture; base URLs on the overview only.
- Redesigned after UI reviews: list cards; a catalog page organized by provider, with models inside their primary provider's card and details in a drawer; defaults for the provider name, model ID and display name; a new catalog page in three steps; each change on a stored catalog saved at once.
- Cache durations only if verified at `v13.3.0`, otherwise none (none given, see Docs). On-behalf-of requirements only as verified in Grafana source (see Docs).
