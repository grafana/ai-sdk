# Design

## Context

- **Sources and versions.** grafana-app-sdk is v0.60.16 (pinned in `ai-gateway/go.mod`). Grafana behavior comes from a local source snapshot (`~/src/grafana`, not a git checkout, so its exact version is unknown) and from the pinned CI image (`GRAFANA_VERSION=nightly`). The Playwright run on that image is the evidence; anything below that depends on Grafana is re-checked there.
- **Manifest routes.** The manifest's `openapi` paths are the custom routes. A path of the form `namespaces/{namespace}/{plural}/{name}/<subresource>` becomes a subresource route of that kind (app-sdk `routes.Route.resolve`). An operation declares an access check with the `x-grafana-declared-authz-resource`, `-subresource` and `-verb` extensions (`routes.authzCheck`).
- **Grafana's route handler** (`pluginroute.routeHandler`), for a kind route:
  1. reads the parent through storage as the caller, which needs `modelcatalogs:get`; a missing object is a Kubernetes `404` `Status`;
  2. runs the declared check through the access client; a refusal is a `403` `Status`, returned before decryption;
  3. decrypts the parent's secure values, cached for one minute per object UID and resource version;
  4. forwards the call over the v3 route protocol with `RouteResource{resource, name, rv, raw, decrypted_secure_values}`.

  Grafana's authorizer leaves custom routes to their declared checks (`authorizeKind`). An access-policy token's own permissions cover only the parent resource and verb.
- **Role registration** (`pkg/router/plugin_roles.go`): a role kind `Kind/route` grants `<group>/<plural>/<route>:<verb>`, which is the action a subresource check asks for.
- **Plugin side:** app-sdk's `routeAdapter` puts the parent into `resource.RouteRequestInfo` (`Plural`, `Name`, `ResourceVersion`, `Parent`, `DecryptedSecureValues`) and dispatches the full request URL to the app's handler.
- **Current Gateway:**
  - The route table has two entries. `AccessMiddleware` calls `AccessPolicy.Decide`. `NewSelector` branches on account access. `discovery.New` takes one lister.
  - `SelectStackCatalog` exists; this design follows the "Follow-ups for #758" list in the archived `2026-11-10-add-stack-catalog-runtime` design.
- **Test host:** `platformhost` serves kind objects from memory with secure plaintext, and forwards every other path with an `httpadapter.RouteInfo` that has no `Parent`.
- **Module policy:** Go code outside `ai-gateway/` must not import the Gateway. `test/integration/testserver` is a library-level server that cannot host the plugin.
- **End-to-end:** stack catalogs refuse `baseURL`, so a catalog call always targets the native `api.anthropic.com` or `api.openai.com` endpoint. The Gateway's model client uses `http.ProxyFromEnvironment`.
- **Plugin environment** (checked on Grafana `main`, which the nightly image builds from, and on tag `v13.3.0`): `pluginconfig.PluginEnvVars` passes the whole Grafana environment through go-plugin when `SkipHostEnvVars` is off. When it is on, it appends `envvars.PermittedHostEnvVars`: `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` (each in both cases), `SSL_CERT_FILE` and `SSL_CERT_DIR`. Either way the plugin receives the proxy and certificate variables.

## Goals / Non-Goals

**Goals:**

- Both catalog routes are served with the same host composition as the namespace routes: route table, access decision, telemetry, runtime and ProviderWire adapter.
- Decrypted values are used only after the parent check, and only for that request.
- Every acceptance item has CI evidence, or the reason it can't and the substitute evidence.

**Non-Goals:**

- Frontend changes, and Cloud enablement of secure values for plugin kinds (#460).
- Caching parsed catalogs across requests.
- Per-stack Agent Observability export.
- Extension ownership and source (#762) and caller identity metadata (#763).

## Decisions

### Manifest: two `openapi` paths with declared checks, and one role

```cue
"/namespaces/{namespace}/modelcatalogs/{name}/config": get: {
	operationId: "getModelCatalogConfig"
	"x-grafana-declared-authz-resource":    "modelcatalogs"
	"x-grafana-declared-authz-subresource": "config"
	"x-grafana-declared-authz-verb":        "get"
	responses: default: description: "ProviderWire discovery response"
}
"/namespaces/{namespace}/modelcatalogs/{name}/language-model": post: {
	operationId: "createModelCatalogLanguageModel"
	"x-grafana-declared-authz-resource":    "modelcatalogs"
	"x-grafana-declared-authz-subresource": "language-model"
	"x-grafana-declared-authz-verb":        "get"
	responses: default: description: "ProviderWire language-model response"
}
roles: "aigateway:modelinvoker": {
	title: "Model invoker"
	kinds: [
		{kind: "ModelCatalog", verbs: ["get"]},
		{kind: "ModelCatalog/language-model", verbs: ["get"]},
		{kind: "ModelCatalog/config", verbs: ["get"]},
	]
}
roleBindings: editor: ["aigateway:modelinvoker"]
```

- **Checks are `get` on the route's own subresource.** Users then need the route grant, which only `modelinvoker` gives. A Viewer, who reads through `catalogreader`, is refused before the parent's values are decrypted. An access policy needs only `modelcatalogs:get`, because its token check covers the parent resource and verb.
- **`get`, not `create`, on the POST route.** Invoking a catalog reads its configuration, and the role table keeps invokers to read permissions.
- **Role naming.** The `aigateway:<kind><verb>` naming rule is relaxed to "lowercase letters and digits", because `modelinvoker` names a capability, not a kind and verb. The hyphen-free form still matches app-sdk's `^aigateway:[a-z0-9]+$` role pattern.
- **Alternative considered:** a check on the parent alone (`modelcatalogs:get`). Rejected: every Viewer could then invoke catalogs and spend the stack's provider keys.
- **Alternative considered:** the deprecated per-kind `routes` CUE field. Rejected: Grafana migrates it to `openapi` paths and `ValidateManifest` refuses it.

### Route table: two entries with their own labels and access decision

- `route` gains a `catalog bool` field. Catalog entries are `GET /modelcatalogs/{name}/config` (label `catalog_config`) and `POST /modelcatalogs/{name}/language-model` (label `catalog_language_model`), under the same prefix and host-error writer.
- `registerRoutes` wraps a catalog entry in `parentCatalog`, then `AccessMiddleware` with `AccessPolicy.DecideCatalog`; other entries keep `Decide`. The table test and the manifest test assert the four pairs.
- `languageModelPath` is reused, so the ProviderWire handler always sees its fixed path.
- The same runtime serves both kinds of route: the selector branches on account access, so one runtime per adapter remains.

### `catalog` access decision

```go
const CatalogAccounts AccountAccess = "catalog"

func (policy AccessPolicy) DecideCatalog(info types.AuthInfo, namespace string) Caller
```

- A caller is refused when the namespace doesn't match, as in `Decide`.
- Otherwise it gets `CatalogAccounts` when it has a user or service-account identity actor, or a direct caller service identity:
  - `Identity` is the identity actor, as today;
  - `attributedService` is the direct caller's service identity when present.
- `Decide` keeps its behavior for the namespace routes.
- The access-decision metric gains `decision="catalog"`.
- **Alternative considered:** a route flag inside `Decide`. Rejected: a separate method keeps the namespace path byte-for-byte unchanged and makes the table test explicit.

### Parent check and catalog carrier

`parentCatalog(next)` runs before the access decision and before the body is read:

1. Read `resource.RouteRequestInfoFrom(ctx)`. A missing parent writes `host.ErrorInternal`.
2. Require `Plural == "modelcatalogs"`, `Name == r.PathValue("name")` and `Namespace == r.PathValue("namespace")`. A mismatch writes `host.ErrorInternal`.
3. Decode `Parent` with the generated `v1alpha1` codec, and require `apiVersion` (group and version), `kind`, `metadata.namespace` and `metadata.name` to match the route. A mismatch writes `host.ErrorInternal`. Bytes that don't decode store a catalog marked undecodable.
4. Store `service.StackCatalog{Name, Spec, Keys: DecryptedSecureValues}` in the request context, unexported and set only by this middleware.

- `service.StackCatalogFromContext` reads it. The selector and the lister turn an undecodable catalog into `ErrCatalogConfiguration`.
- Grafana builds the parent from the route itself, so a mismatch means a platform defect, not a caller error. That's why it is internal, and not permission, which would suggest the caller could fix it.
- **Alternative considered:** passing the catalog through `RequestSelector`'s arguments. Rejected: it would reshape the selector signature that #762 also touches.

### Invocation: one selector branch

`NewSelector` gains, before the default refusal:

```go
case caller.AccountAccess() == gatewayauth.CatalogAccounts:
	stack, ok := StackCatalogFromContext(ctx)
	if !ok {
		return execution.Selection{}, execution.ErrAccountAccess
	}
	return SelectStackCatalog(stack, id, gateway, stackOptions)
```

- `NewSelector` takes the stack-catalog options, which carry the shared model client.
- A catalog edit applies on the next request without plugin state. A changed resource version gives a new decrypted map, and Grafana caches decrypted values for at most one minute per version. The README states the one-minute bound for a key rotated without an object change.

### Discovery: a stack lister and the host category

- `service.StackCatalogLister` implements `catalog.ModelLister`. It reads the context catalog, runs `stackcatalog.Parse` and returns `ModelInfo` rows: canonical ID, name, description, aliases, and candidates with provider instance = provider name, provider = provider type and model ID. It constructs no model. Any failure returns `ErrCatalogConfiguration`.
- `discovery.New` gains the catalog route's lister. A handler serves configured access with the operator lister and catalog access with the stack lister, which keeps one handler type.
- `errors.Is(err, execution.ErrCatalogConfiguration)` writes `host.ErrorCatalogConfiguration`; ProviderWire maps it to the existing `safeCatalogConfiguration` document.

### Test host: subresource routes with a parent

- `kindStore.kindPath` also recognizes `{plural}/{name}/<subresource>`. For a declared kind route, the host reads the stored object (`404` when missing). It forwards with `RouteInfo{Path: <subresource>, Parent: RouteResource{resource, name, rv, raw: stored JSON with secure names, decrypted_secure_values: plaintext held in memory}}`.
- The host keeps no RBAC, declared checks or decryption cache; those are Grafana's and are proven end to end.
- `plugin-host --manifest` already gives it the kinds; routes come from the manifest's `openapi` paths.

### Cross-language scenarios in `ai-gateway/test/providerwire-v4`

- The suite already launches the built plugin through `plugin-host --manifest` with the pinned `@ai-sdk/gateway` client and the Go `providers/grafana` client. A `gatewaytest` build trusts a test CA through `nativeProxyEnvironment`, and `startNativeProviderProxy` intercepts `api.anthropic.com:443`.
- Scenarios:
  - create a catalog through the test host (admission and secure values);
  - with `baseURL = .../modelcatalogs/team`, list `config` with both clients;
  - run one streamed and one unary call;
  - assert the fake native provider received the catalog's key, and the assembled stream parts.
- Chunks are parsed with the suite's existing registered-client paths. Parsing with `parseJsonEventStream` and `uiMessageChunkSchema` would apply to UI-chunk scenarios, but ProviderWire isn't UI chunks, and this suite owns ProviderWire client evidence.
- **Deviation from the issue**, which asks for these scenarios under `test/integration/`. That harness is a library-level Go test server, and module policy forbids Go code outside `ai-gateway/` from importing the Gateway, so it cannot host the plugin. This suite already runs both registered clients against the built plugin through the plugin protocol. The PR description records the deviation.

### End-to-end plan (Playwright, CI)

**Infrastructure** in `docker-compose.e2e.yaml`; no production change:

- The proxy is e2e-only and ephemeral. No production code path, build tag or committed certificate changes.
- `scripts/e2e.sh` generates an ephemeral self-signed certificate and key for `api.anthropic.com` and `api.openai.com` with `openssl` into a temporary directory, which is mounted read-only and removed with the environment.
- The proxy reuses the existing mechanism rather than adding a second one. `startNativeProviderProxy` in `ai-gateway/test/providerwire-v4/native-provider-proxy.ts` takes the certificate and key as parameters; the existing suite keeps passing its fixture.
- A `native-proxy` service runs a small entrypoint over that module with Node 24's built-in TypeScript support. It accepts `CONNECT api.anthropic.com:443`, terminates TLS with the run-time certificate and forwards to `fake-anthropic`; it refuses every other authority.
- The Grafana service sets:
  - `HTTPS_PROXY=http://native-proxy:8443`;
  - `NO_PROXY=localhost,127.0.0.1,fake-anthropic,native-proxy`;
  - `SSL_CERT_FILE` to the certificate. Go still loads the system certificate directories.
- `fake-anthropic.mjs` answers with text that names a short SHA-256 prefix of the received `x-api-key`. Tests compare that prefix without the key appearing in any response.

**Specs** (`tests/catalog-routes.spec.ts`, using the existing role-token fixture):

| Acceptance | Test |
|---|---|
| Viewer, Editor and Admin allow and deny | Viewer: `403` on both routes with a Kubernetes `Status`, and the plugin log has no catalog call. Editor and Admin: `200` on `config` and on unary and streamed `language-model`. `/api/access-control/user/permissions` shows `modelcatalogs/language-model:get` and `modelcatalogs/config:get` for Editor only. |
| Decrypted keys reach the provider; edits apply on the next request | Create with key K1: the response names K1's hash prefix. `PUT` the key to K2 (a new resource version): the next call names K2's. Change a model's primary: the next call reaches the new backend model. |
| Discovery | `config` lists the catalog's model, aliases and candidates, and is identical in shape to the namespace route projection. |
| Grafana's `403` and `404` | An unknown catalog name gives `404`, and the Viewer call `403`, both Kubernetes `Status` JSON. The README documents both, and `test/providerwire-v4` proves each client surfaces them as errors. |
| Namespace routes unchanged | `routes.spec.ts` unchanged; the namespace `config` still lists only the operator catalog. |
| Keys never logged | The sentinel check in `scripts/e2e.sh` also covers the route keys. |

**What CI cannot run, and the substitutes:**

- **Access-policy tokens.** Self-hosted Grafana has no access policies, so the `modelcatalogs:get` acceptance can't run against Grafana in CI. Substitutes:
  - `DecideCatalog` unit tests for an access policy alone and acting for a user;
  - a test-host test where `AddAccessPolicy` and on-behalf tokens invoke both routes;
  - Grafana's own coverage of token permission checks (`authorizeTokenPermissions`).

  The README states it as the Cloud path, verified when #460 enables secure values there.
- **The internal parent check.** Grafana never sends a mismatched parent, so this is unit and test-host evidence only.
- **The catalog configuration document.** Admission refuses invalid catalogs, so it's unreachable on Grafana. Covered by Go and test-host tests.

## Test plan (Go and TypeScript)

- `auth`: a table test for `DecideCatalog` over user, service account, access policy alone, access policy for a user, anonymous, and namespace mismatch, in both modes. `Decide` tests stay unchanged.
- `gateway`:
  - route-table and manifest tests for the four pairs and the labels;
  - `parentCatalog` tests: missing, plural, name or namespace mismatch, decode failure, and no body read;
  - harness tests: invocation with the key reaching a fake native provider, alias and fallback, `byok` refused, and an edit through the test host taking effect next call;
  - discovery, plus an operator-catalog-only model returning not found;
  - metrics `decision="catalog"` and labels, with no catalog name in metrics.
- `service`: `StackCatalogLister` rows, error mapping, zero constructions; the selector branch with a missing carrier refused.
- `discovery` and `providerwire/v4`: the host category and document bytes.
- `platformhost`: subresource forwarding, `404`, parent fields, decrypted values.
- `test/providerwire-v4`: the scenarios above, plus Kubernetes `Status` `403`/`404` consumption by both clients.

## Risks / Trade-offs

- [Declared-check support in the minimum Grafana version] → v13.3.0 has `Kind/route` roles and decrypted values for kind routes. Declared-check extensions are confirmed only in the source snapshot, so the 13.3.0 one-off run covers this suite (see Follow-ups), and the README states the version it passes on.
- [Proxy and `SSL_CERT_FILE` affect Grafana's own outbound calls in e2e] → `NO_PROXY` covers local services, and the system certificate directories still load. This applies to the e2e override only.
- [Attribution emits the direct caller service for cloud sessions] → in Cloud, a session's direct caller may be the platform front door, so logs name it. It is platform-signed and bounded; #763 revisits it.
- [#762 merge] → `NewSelector` gains a branch and `discovery.New` a parameter. Neither touches `ModelComposer` or `Source`.
- [The one-minute decrypted-value cache] → a key rotated without changing the object can take up to a minute to apply, which the README documents. Admission makes every key change through the API an object update, which changes the resource version.

## Migration Plan

Additive. Existing namespace routes, settings and roles are unchanged. Rolling back removes the routes and role; stored catalogs remain readable through the kind.

## Decisions recorded at review

- Cross-language scenarios live in `ai-gateway/test/providerwire-v4`, not `test/integration/` (module policy; see that section).
- A parent mismatch returns the internal host error.
- Catalog access emits the direct caller's service identity when present.
- The POST route's declared verb is `get`.
- Role names are lowercase letters and digits; the role stays `aigateway:modelinvoker`.

## Follow-ups

- **#763:** revisit catalog-access attribution with caller identity. In Cloud, a session's direct caller may be the platform front door.
- **The one-off `GRAFANA_VERSION=13.3.0` CI run from #770** should also cover `tests/catalog-routes.spec.ts`, which confirms declared-check support in the minimum version.
- **Cloud:** verify the access-policy acceptance once #460 stores secure values for plugin kinds.
