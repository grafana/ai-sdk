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
| `catalogs/new` | Catalog form, create | `create` |
| `catalogs/edit/:name` | Catalog form, edit or read-only | `get`; `update` to edit |

- `plugin.json` adds a "Model catalogs" include at `/a/%PLUGIN_ID%/catalogs` with `addToNav`. It also sets `action: aigateway.ext.grafana.app/modelcatalogs:get`, so Grafana shows the nav entry only to readers. The page checks the permission anyway, because the include action gates navigation, not the URL.
- `ROUTES` gains `Catalogs`. `App.tsx` adds the three routes before the catch-all. Sub-pages pass `pageNav` to `PluginPage` for breadcrumbs.
- **Alternative considered:** `catalogs/:name` together with `catalogs/new`. Rejected: `new` is a valid catalog name and would be unreachable. `edit/:name` follows Grafana's data source pages (`/datasources/new`, `/datasources/edit/:uid`), which also serve read-only users.

### API module

`src/api/modelCatalogs.ts` wraps `getBackendSrv()`. It offers list, get, create, update and delete. Every request is sent with `showErrorAlert: false`, so the page owns error display and Grafana shows no toast as well.

- **Base path:** `${config.appSubUrl}/apis/${API_GROUP}/${API_VERSION}/namespaces/${config.namespace}/modelcatalogs`. Grafana core uses the same pattern in `getAPIBaseURL`.
- **Types:** the generated `ModelCatalog`, `Spec` and `InlineSecureValue`; no hand-written duplicates.
- **`errorMessage(err)`** returns `err.data.message` for a fetch error whose body is a `Status`, otherwise `err.data.message ?? err.statusText ?? err.message`. It doesn't rewrite the message.
- **Alternatives considered:**
  - RTK Query or the `@grafana/api-clients` packages: they aren't dependencies;
  - a generic Kubernetes client: one kind doesn't justify one.

### Permissions

`src/permissions.ts` exports the four action names and `useCatalogPermissions()`, which returns `{read, create, update, delete}` from `hasPermission`.

- Every control asks this one helper. Nothing checks `orgRole`.
- Boot data is loaded when the page loads, so a permission change shows after a reload. Grafana stays authoritative: a refused write shows its `403` like any other error.
- **Alternative considered:** probing with requests, such as a dry-run create. Rejected: it adds a write call for every view, and `hasPermission` is the documented plugin API that core features use.

### Form model and key behavior

The form state holds:

- the catalog `name`;
- a list of provider rows: `name`, `type` (`anthropic` or `openai`), and `key`, a string the writer typed, empty when untouched;
- a list of model rows: `id`, `name`, `description`, `aliases`, `primary {provider, model}` and `fallback[]`.

Rows instead of maps, so a name can be edited and duplicates can be detected. The page converts between the rows and `Spec`. Loading an object maps its `spec` into rows. It records the stored secure keys, the keys of `secure`, never their names, and the object's `metadata`. It doesn't hold any key.

`secureChanges(storedKeys, providers)` is a pure function that builds the `secure` map of a write:

| Provider in the form | Name stored | Key typed | Sent |
|---|---|---|---|
| yes | yes | no | nothing (kept) |
| yes | yes | yes | `create` |
| yes | no | yes | `create` |
| yes | no | no | nothing; admission reports the missing key |
| no | yes | — | `remove: true` |

- A rename therefore removes the old key and needs a new one, because Grafana refuses reusing a secret name under another key.
- The update sends the loaded `metadata` unchanged, including `resourceVersion`, so a concurrent change gives `409`. It sends the new `spec` and `secure` only when non-empty. The create sends `apiVersion`, `kind`, `metadata.name`, `spec` and `secure`.
- **Key field:** `SecretInput` from `@grafana/ui`. It shows as configured when the row's current name is a stored key and no key has been typed. Reset clears that state, so the writer can type a replacement. A reset left empty sends nothing, so the stored key stays: Grafana doesn't allow a provider without a key anyway. The typed value is never written anywhere except the request body. After a successful save, the page navigates to the list, which drops the form state.
- **Read-only mode** applies without `update`, or without `create` on `new`. Inputs are disabled, and Save, Delete and the add and remove row controls are absent. Keys show only as configured or not.
- **Local checks:** Save is refused only when two provider rows share a name or two model rows share an ID. These are the only invalid states the map-shaped body can't carry. Empty names, unknown provider references, alias collisions and policy violations go to admission.
- **Alternative considered:** a JSON spec editor (`CodeEditor`) with key fields per provider. Rejected:
  - users have to know the schema;
  - keys and providers can drift apart in the editor;
  - Jest testing of Monaco is awkward.

  The structured form covers the whole current spec, which admission limits to `type` per provider.

### List and delete

- The list is an `InteractiveTable` with:
  - the name, linking to `edit/:name`;
  - the provider names and types;
  - the model IDs;
  - a Delete action for `delete` holders, behind a `ConfirmModal`.
- With `create`, a "New catalog" `LinkButton` appears in the page actions.
- When the namespace has no catalogs, an `EmptyState` appears, with the create call to action only for `create` holders.
- Load, delete and save errors are an `Alert` with severity `error` and the `errorMessage` text, shown above the content. The form keeps its state on error.

### Overview

- A "Model catalogs" block under "Endpoint", rendered only when `read` is true. It lists catalogs once on mount.
- For each catalog it shows the name and a `CodeSnippet` of `catalogBaseURL(endpoint, name)`, which is `${gatewayEndpoint(...)}/modelcatalogs/${encodeURIComponent(name)}`.
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
  - a bullet on propagation: edits apply on the next call, but a removed role or a revoked token can keep working for a while, because Grafana caches permissions and the tokens it signs for the plugin. Cite durations only when the code at the Grafana `v13.3.0` tag confirms them. The snapshot shows a 60-second permission cache and 10-minute ID tokens;
  - the Install step's overview sentence also names catalog URLs.
- **`ai-gateway/src/README.md`** (the plugin catalog listing): one sentence on the catalog page.
- **`ai-gateway/docs/model-catalog.md`:** a short "Stack catalogs" section near the top. It says this package builds catalogs inside a Go service, while stack catalogs are `ModelCatalog` resources the Gateway serves per stack, and links to the README section.
- **`docs/providers/grafana-gateway.md`:** a new "Offer Grafana models and customer catalogs" section after "Discover and select public models". It covers:
  - one client per route, since a client has one base URL;
  - storing the customer's choice (catalog name and model ID) in the consuming plugin's settings;
  - in Cloud, operator models need a backend in `access_internal_services` using token exchange;
  - calling the catalog route on behalf of the user with `WithOnBehalfOf`, so Grafana checks the user's access. Verify against Grafana source what the user needs before documenting it;
  - why falling back to Grafana models is discouraged: the customer chose their provider and account, a silent fallback sends prompts elsewhere and hides a broken catalog, and the Gateway never crosses sources itself.

  Any Go snippet reuses calls already in the guide, so no new API is invented.
- **`docs/guides/gateway-authentication.md`:** the overview sentence names catalog URLs.
- **Architecture walkthrough and glossary:** they don't describe the frontend. Recheck them after implementation and change them only on drift.

### Tests

**Jest**, with `@grafana/runtime` mocked for `getBackendSrv`, `config` and `hasPermission`:

| Acceptance | Tests |
|---|---|
| Admin creates, edits, deletes; keys write-only | `secureChanges` table (each row above, rename, re-add). The API module sends `showErrorAlert: false` and builds URLs with `appSubUrl` and namespace. The create body carries `create`. The edit body omits unchanged keys, keeps `metadata.resourceVersion`, and sends `create` after a reset and `remove` for a deleted provider. The key input is configured after load and the typed key isn't in the DOM after save. Delete calls the API only after confirmation. |
| Viewer and Editor read-only | A permission matrix that sets permissions independently of `orgRole`: Admin without `update` sees read-only, and Editor with `create` sees "New catalog". Without `get`, no request and no content. |
| Admission errors as returned | A rejected save with a `Status` body shows its `message` verbatim and keeps the input. A duplicate provider name blocks the request. |
| Overview base URLs | Lists catalog URLs, including `appSubUrl` and namespace encoding. No request without `get`. Empty state. A load error doesn't hide the endpoint. |
| Routing | `App` renders list, new, edit and overview at their paths. |

**Playwright:**

- `tests/catalog-page.spec.ts` is new.
- `fixtures.ts` gains a worker-scoped `users` fixture. It creates a Viewer and an Editor login user through the admin API, deletes them afterwards, and offers `pageAs(role)`, a new browser context logged in through `POST /login`. Admin tests use the default admin page.

| Acceptance | Test |
|---|---|
| Admin create | Fill the form with a sentinel key and save. The list shows the catalog. The page content never contains the sentinel, and an API `GET` shows only `secure.<p>.name`. |
| Admin edit, keys kept | Reopen: the key shows as configured, and the page has no sentinel. Change a model name and save. The intercepted `PUT` body has no `secure` entry, and the API's secure name is unchanged. Replace the key: the `PUT` carries only `create` for that provider. |
| Admin delete | Delete and confirm. The API returns `404` and the list drops it. |
| Admission errors | Save a provider named `Anthropic`. The alert shows admission's message, and the page has no sentinel. |
| Concurrent edit | Open edit, update through the API, then save. The page shows the `409` message. |
| Viewer read-only | `pageAs('Viewer')`: the list shows the catalog, with no New or Delete. The edit page has disabled inputs and no Save. `catalogs/new` is read-only. |
| Editor read-only | The same as `pageAs('Editor')`. |
| Overview | Create a catalog through the API. The overview shows `.../namespaces/default/modelcatalogs/<name>`. |

- "Base the UI on permissions, not roles" can't be shown end to end on OSS, which can't grant `catalogwriter` to an Editor. The Jest permission matrix carries it, and the Viewer and Editor runs prove the bound roles.
- The existing `e2e.sh` sentinel log check also covers these keys, because the spec uses the same sentinel prefix.

## Risks / Trade-offs

- [Boot-data permissions are loaded once per page load] → a role change shows after a reload. The server refuses stale writes with `403`, and the page shows the error.
- [Include `action` support or plugin validator warnings] → if the validator or Grafana 13.3 rejects the include action, fall back to the default Viewer include and keep the in-page check.
- [A future spec field the form doesn't model would be dropped on save] → the form covers the whole current schema. Adding a kind field must extend the form in the same change, and a Jest round-trip test of a full spec catches drift.
- [Grafana list limits] → the page makes one list call. A stack is expected to hold few catalogs; paginate if the platform starts truncating.
- [Overview in Grafana Cloud], where catalogs with keys are refused → the list is empty or fails. Either case renders inline without affecting the quick start.
- [Browser login users in e2e depend on the login form] → the compose base keeps the login form enabled. plugin-e2e already logs in `admin` the same way.

## Migration Plan

Additive frontend change. Rolling back removes the page and the overview section; stored catalogs are unaffected.

## Open Questions

- The exact cache durations to cite for removed roles and revoked tokens. Resolve them from the Grafana `v13.3.0` source while writing the docs, or keep the wording without durations.
