# Design

## Context

- Registered baseline: not parity-sensitive (see proposal). Reference platform sources: grafana-app-sdk v0.60.16 (pinned in `ai-gateway/go.mod` and `.config/app-sdk/generate-kinds.mjs`) and Grafana `v13.3.0`.
- `kinds/manifest.cue` declares no kinds and no roles. Because it declares no roles, the generator emits default `aigateway:reader|editor|admin` roles with no kinds; Grafana registers nothing for a manifest without kinds (`pkg/router/plugin_roles.go`).
- Grafana (`pkg/services/pluginsintegration/pluginroute`) serves a manifest without kinds through a routes-only handler, and a manifest with kinds through a full API server whose `ServeMux` wraps the same route mux inside the API server's filter chain. Adding the first kind therefore changes the handler that serves the namespace routes.
- Grafana's kind store (`pkg/services/apiserver/kindstore`) prunes undeclared fields in `PrepareForCreate/Update`, before validating admission, then validates against the schema. Validating admission receives the incoming object with `secure.*.create` plaintext and, on update, the stored object with names only. Unified storage then applies the inline secure value rules (`pkg/storage/unified/apistore/secure.go`): `create` makes an owned secret, `remove` drops a key, an omitted `secure` map or key keeps the stored value, an unchanged `name` is kept, a different `name` replaces the stored value.
- `folderScoped: false` disables folder support in storage, so a folder annotation is refused.
- app-sdk v0.60.16: `simple.AppManagedKind{Kind, Validator}`; `simple.App` starts informers only for kinds with a reconciler or watcher; `appadapter.AdmissionAdapter` decodes the objects with the kind codec and returns a plain error as a `403` denial, an `APIStatus` error with its own code. `simple.App.Validate` logs the returned error. `resource.RawSecureValue` redacts itself in formatting and JSON.
- `config.File.Validate` interleaves per-provider type checks with the `apiKeyEnv` check, then checks models. Map iteration makes provider order random, but within one provider the type error precedes the key error, and provider errors precede model errors.
- The CUE manifest definition declares role names as `^aigateway:[a-z0-9]+$` and applies `#Role` only to labels that match it. Grafana validates only the `fixed:` prefix of the registered role name.
- `pkg/internal/...` is importable only below `ai-gateway/pkg/`; the test host in `ai-gateway/test/platformhost` cannot import the generated manifest package.

## Goals / Non-Goals

**Goals:**

- One shared place for later work: spec conversion and stack checks (#757), the kind and roles that catalog routes extend (#758), the `ManagedKinds` list, `roles` map and test host kind support a second kind reuses (#764).
- The operator catalog's startup validation keeps its behavior, messages and tests.

**Non-Goals:**

- Building or serving models from catalogs, catalog routes, the `modelinvoker` role, decryption, per-request validation bounds (#757, #758).
- Schema validation, pruning or RBAC in the test host; those are proven against Grafana.

## Decisions

### Kind schema

`kinds/modelcatalog.cue`, referenced from `v1alpha1.kinds`:

```cue
modelCatalogKind: {
	kind:         "ModelCatalog"
	folderScoped: false
	secure: [{key: "*"}]
	validation: operations: ["CREATE", "UPDATE"]
	schema: {
		#Target:   {provider: string, model: string}
		#Provider: {type: string, baseURL?: string, providerName?: string, supportsStructuredOutputs?: bool}
		#Model:    {name: string, description?: string, aliases?: [...string], primary: #Target, fallback?: [...#Target]}
		spec: {providers: [string]: #Provider, models: [string]: #Model}
	}
}
```

- Field names and requiredness follow `config.File`, so an operator document without `apiKeyEnv` is a valid spec.
- `type` stays a string. An enum would move the type error to Grafana's schema message, and #757 validates stored specs per request with the same Go check anyway.
- The unsupported fields are optional, so the generated Go type has pointers and admission refuses their presence, including `false` and `""`. Without them in the schema, pruning would drop `baseURL` before admission and a key would go to the default endpoint.
- `apiKeyEnv` is not in the schema: pruning it is harmless, and the missing secure key is reported instead.
- `secure: [{key: "*"}]` because provider names are tenant-chosen. The one-to-one check replaces a fixed key list.
- No bounds in the schema: the Go shape check owns them, so admission and #757 apply one implementation.

### Manifest roles

```cue
roles: {
	"aigateway:catalogreader": {title: "Model catalog reader", kinds: [{kind: "ModelCatalog", permissionSet: "viewer"}]}
	"aigateway:catalogwriter": {title: "Model catalog writer", kinds: [{kind: "ModelCatalog", permissionSet: "editor"}]}
}
roleBindings: {viewer: ["aigateway:catalogreader"], admin: ["aigateway:catalogwriter"]}
```

- One shared `roles` map and `roleBindings` block; #758 adds `aigateway:modelinvoker` and #764 its middleware roles to the same blocks.
- Declaring roles removes the generated default roles. They granted nothing and Grafana registered none, so no caller loses access.
- Grafana registers them as `fixed:aigateway.ext.grafana.app:aigateway:catalogreader|catalogwriter`. Basic roles inherit, so Editors and Admins also read.
- No hyphens: a hyphenated label escapes the SDK's `[a-z0-9]+` pattern, so its role body is never validated against `#Role`, and it would break once the struct is closed, when renaming fixed role IDs would break role assignments. Later roles follow the same convention (`modelinvoker`, `middlewarereader`, `middlewarewriter`).
- A manifest test pins the role names, permission sets, bindings and the rendered `fixed:` IDs.

### Validation split

`config` keeps the operator path and exposes the shared shape check:

- `File.Validate()` (operator, unchanged): providers present; per provider, the existing name and type checks, then `apiKeyEnv`; then models. Implemented as the private shape walk with a per-provider hook that checks `apiKeyEnv`, so error precedence is the same.
- `File.ValidateShape()`: the same walk without a key hook. It owns models, aliases, references, cardinality and text bounds for both catalogs.
- `ParseModelCatalog`, `ResolveProviderSecrets` and every existing message and test stay unchanged.

A new `pkg/internal/stackcatalog` package owns the stack catalog:

- `Parse(spec v1alpha1.ModelCatalogSpec, keys []string) (config.File, error)` runs, in order: policy on the spec (DNS label names, `anthropic`/`openai` only, no unsupported field present), conversion to `config.File`, `File.ValidateShape`, then the key check (every provider has a key, every key names a provider).
- Policy runs on the spec because conversion loses field presence.
- `keys` is a set of secure keys, not values, so admission passes the merged keys and #757 passes the keys of its decrypted values, then resolves providers from the values itself.
- Messages use a `modelcatalog:` prefix and name field paths and identifiers only. Shape messages are the shared `config:` messages.

Alternative considered: put conversion and policy in `config`. Rejected: `config` would import generated kind types and own platform-resource semantics, while `stackcatalog` keeps the operator package unchanged apart from the shape export.

### Admission algorithm

`stackcatalog.Validator` implements `simple.KindValidator`. For `CREATE` and `UPDATE`:

1. Type-assert `req.Object` (and `req.OldObject` on update) to `*v1alpha1.ModelCatalog`; anything else is refused with a fixed message.
2. Merge secure keys. Start from the stored keys on update (empty on create), then for each incoming key:
   - `remove` with `create` or `name`: refused;
   - `remove`: drop the key;
   - `create` without `name`: keep the key;
   - `name` without `create`: keep the key only on update when it equals the stored name under that key; otherwise refused as a shared secret reference, including the stored name of another key (a swap);
   - empty entry, or `create` with `name`: refused.
3. `Parse(spec, mergedKeys)`.
4. Any failure returns `apierrors.NewBadRequest(message)`, so Grafana answers `400` with the message, and RBAC refusals (`403`) stay distinguishable.

The validator reads `create` only through `IsZero`, never formats a secure value, and builds every message from field paths, keys and spec identifiers. A sentinel unit test checks messages and the plugin log handler output.

Alternative considered: refusing every `name`, as the issue states. Rejected: a read-modify-write update and any patch carry the stored names back, so every such update would be refused. Repeating the stored name references the catalog's own secret, which storage accepts as owned; any other name is still refused.

### App wiring

`Gateway.Provider()` passes `ManagedKinds: []simple.AppManagedKind{{Kind: v1alpha1.ModelCatalogKind(), Validator: stackcatalog.Validator{}}}` to `simple.NewApp`, next to `Handler`, for running and idle Gateways alike. #764 appends its kind with its own validator. No reconciler or watcher, so no informer starts and the kubeconfig is not used. An idle Gateway has no authenticator, so the plugin runtime refuses its admission reviews. A unit test runs `simple.App.ValidateManifest` on the committed manifest.

### Test host kind support

- `platformhost.Config` gains the manifest (`app.ManifestData`). Go tests below `pkg/` pass `manifestdata.LocalManifest()`; the `plugin-host` command reads `app-sdk-manifest.json` next to the plugin binary.
- `Host.Handler` takes a `clientv3.Client` (routes and admission). Paths `namespaces/{ns}/{plural}` and `namespaces/{ns}/{plural}/{name}` of a declared kind are served from an in-memory store: `POST` create, `GET` get, `PUT` update, `DELETE` delete. Everything else is forwarded to routes as today.
- Each write is authenticated and namespace-checked like a route call, then sent as an `AdmissionReview` (object and old object as JSON) when the kind declares that operation. A denial is returned with its status. On allow, secure values follow the inline rules: `create` becomes a generated name with the plaintext kept in host memory (for #758's decrypted values), `remove` drops, omitted keys and unchanged names are kept. Reads return names only.
- No schema validation, pruning, RBAC or resource versions beyond a counter.
- The secure value store is documented as test-only, holds plaintext only in memory, and never writes it to logs or responses.

### Proving the namespace routes unchanged

- Go: the manifest test keeps asserting the two namespaced paths and methods; the route table test is unchanged.
- Grafana: a route spec under `ai-gateway/tests/` covers `GET config` (status, content type, configured model), unary and streaming `POST language-model` against a deterministic fake Anthropic provider, the Gateway error document for an unknown model, and `405` for an undeclared method. It is run first on the base branch build (routes-only manifest) and then on this change, both green, and both runs are reported.
- The fake provider is a small deterministic Node SSE server under `ai-gateway/tests/`, started as a compose service in an `e2e` profile, with one extra provider and model in the provisioned catalog pointing at it through `baseURL`. `outbound.ValidateEndpoint` accepts any absolute `http` or `https` URL with a host, and the Go route tests already use plain-HTTP fakes, so no production check changes. It is end-to-end infrastructure, not provider evidence, and stays out of `test/conformance`.

### End-to-end tests

New Playwright specs under `ai-gateway/tests/`, with a fixture that creates Viewer, Editor and Admin service accounts and tokens through the Grafana API as the anonymous Admin, and deletes created catalogs after each test:

| Acceptance | Test |
|---|---|
| Roles register and bind | `/api/access-control/user/permissions` for each token: Viewer and Editor hold `aigateway.ext.grafana.app/modelcatalogs:get` only, Admin also `create`, `update`, `delete`. Viewer and Editor can list and get but get `403` on create; Admin creates. |
| Saves without a folder | Admin creates without annotation (`201`); with `grafana.app/folder` it is refused. |
| Secret stored, names on read | Create with `create` sentinels; the create response, get and list contain `secure.<p>.name` only and no sentinel; the named secure value exists in the secret API. |
| Update without `secure` | `PUT` a changed spec without `secure`: `200`, names unchanged. Read-modify-write with stored names: `200`. |
| Removing a used key | `remove` on a key whose provider stays: `400`. Removing provider and key together: `200`. |
| `baseURL` and `name` refused | Create with `baseURL`: `400`. Create with `secure.<p>.name`: `400`. Update with another name: `400`. |
| Errors without keys | Every refusal carries sentinel `create` values; no response body contains them. `e2e.sh` fails the run when the Grafana container log contains a sentinel. |
| Routes unchanged | The route spec above. |

Run locally: `mise run build-ai-gateway-plugin`, then `GRAFANA_VERSION=<CI pin> mise run test-ai-gateway-e2e`, then once more with `GRAFANA_VERSION=13.3.0` to back the minimum version.

### Minimum Grafana version

Grafana `v13.3.0` (released 2026-10-27) is the first tag containing: kinds and manifest roles registered from plugin manifests (#132037), `Kind/route` role kinds (#135067), folder scoping from `folderScoped`, the full-API-server/routes-only split, decrypted secure values for kind subresource routes (#133535) and local plugin token signing (#135008). Inline secure values have no feature toggle in `v13.3.0`. The toggles stay `appplugins.loadAppManifest`, `appplugins.registerAPIServer` and `grafana.useRouterMiddleware`, which the README already requires with 13.3. The `13.3.0` end-to-end run confirms it; the README states the result and the Cloud limitation.

### Documentation

- Glossary: "Model catalog kind; `ModelCatalog`; stack catalog", distinguished from the operator catalog and from the existing "Admission control" entry, which means capacity, not Kubernetes admission.
- Gateway README requirements: catalog kind requirements and the Cloud limitation.
- Architecture walkthrough: one sentence that the manifest also declares the catalog kind, served by Grafana with plugin admission, if the current text describes a routes-only manifest.

## Risks / Trade-offs

- [The full API server changes route behavior] → the route spec runs on both builds; a difference blocks the change.
- [A future generator enforces `[a-z0-9]+` role names] → the manifest test fails on upgrade; renaming roles then needs a migration decision.
- [Pruning or schema behavior differs between Grafana versions] → `baseURL` is in the schema, so admission sees it either way; the e2e asserts the refusal on both images.
- [Admission fails closed while the plugin is down or idle] → writes fail, deletes work; documented.
- [Shape messages echo spec identifiers into plugin logs through the SDK's admission log] → identifiers are tenant configuration, not secrets; values of secure fields never enter a message.
- [#757 maps `Parse` errors] → its `catalog configuration` category must use a fixed message and must not forward these messages.
- [#758 needs decrypted values in the test host] → the host keeps created plaintext in memory, keyed by name.

## Migration Plan

No migration. Existing deployments keep the operator catalog; the kind is empty until written. Rollback to a routes-only build leaves stored catalogs in storage unread; Grafana stops serving the kind with the older manifest.
