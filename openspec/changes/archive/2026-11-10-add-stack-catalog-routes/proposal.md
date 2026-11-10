# Proposal

## Why

A stack can store a `ModelCatalog` in Grafana, and the Gateway can build that catalog's models per request. But no route serves them, so callers can neither discover nor invoke a stack's own models. Grafana already offers what a kind subresource route needs. It reads the parent catalog as the caller and runs the route's declared access check. It then decrypts the parent's secure values and passes the object, its resource version and the decrypted values to the plugin. The Gateway has to declare those routes, decide access for them and serve them.

## What Changes

- **Two catalog routes** on the `ModelCatalog` kind, mounted at `namespaces/{namespace}/modelcatalogs/{name}/`:
  - `POST .../language-model`;
  - `GET .../config`.

  Existing clients use `.../modelcatalogs/{name}` as their base URL unchanged, because they append `/language-model` and `/config` themselves.
- **Declared access checks:** each route checks `get` on its own `modelcatalogs/<route>` subresource. Grafana runs the check before decrypting the parent's keys. For an access-policy token, it covers only the parent resource and verb, so `aigateway.ext.grafana.app/modelcatalogs:get` suffices.
- **A new manifest role, `aigateway:modelinvoker`,** bound to Editor. It grants `get` on `ModelCatalog`, `ModelCatalog/language-model` and `ModelCatalog/config`. Viewers keep reading catalogs through `aigateway:catalogreader` but cannot invoke them. Invokers can read catalog specs, never key values.
- **A `catalog` account type** for the catalog routes. It is decided from the verified token and the route namespace only; `access_mode` doesn't apply:
  - a user, service account or access-policy token whose namespace covers the route's is granted `catalog` access;
  - every other caller is refused.
- **The parent check:** the parent object's group, kind, namespace and name must match the route before any decrypted value is used. Otherwise the request fails with the internal host error and does no selection, listing or provider work.
- **Invocation:** the `catalog` branch of the shared selector calls the stack-catalog selection with the parent's spec, its decrypted values and its name. A catalog edit applies on the next request, because Grafana passes the stored object and values on every call.
- **Discovery** of the parent catalog, with the existing ProviderWire projection and no model construction. A catalog that fails its checks gets a new host error category, `catalog configuration`. It uses the same fixed 424 document as the runtime category.
- **Telemetry:**
  - the access-decision counter gains the `catalog` value;
  - the two routes get their own fixed labels, `catalog_config` and `catalog_language_model`;
  - logs and Agent Observability already name the catalog.
- **Test host:** the protocol test host serves subresource routes of declared kinds. It sends the stored object, its resource version and the decrypted values held in its memory as the route's parent, the way Grafana does.
- **Evidence:**
  - Playwright tests against Grafana in CI cover roles, invocation with decrypted keys, edits and Grafana's refusals;
  - the cross-language ProviderWire suite covers discovery and a streamed call through a catalog base URL with both registered clients.
- **Docs:**
  - README: catalog routes, roles, base URL, and Grafana's `403`/`404` responses;
  - Grafana Gateway client guide: the catalog base URL;
  - architecture walkthrough, diagrams and glossary: the catalog account path.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-plugin-host`: the manifest declares four routes; the route table matches them; host-error categories gain `catalog configuration`; discovery lists the parent catalog for `catalog` access; the parent check; the test host sends parent objects and decrypted values.
- `gateway-caller-access`: `catalog` access for the catalog routes, which Gateway roles and declared checks govern; the no-role rule stays scoped to the namespace routes.
- `gateway-model-catalog-kind`: the `aigateway:modelinvoker` role and the role-naming rule. Stored catalogs now serve their own catalog routes, while the namespace routes stay unaffected. Adds the end-to-end evidence for the routes.
- `gateway-stack-catalog-runtime`: the catalog language-model route selects from its parent catalog and its decrypted values.
- `gateway-configured-discovery`: catalog route discovery uses the same projection over the parent catalog.
- `gateway-plugin-telemetry`: the `catalog` access decision.
- `providerwire-v4-unary-runtime`: the `catalog configuration` host error category reuses the fixed 424 document.

## Impact

- **Code:**
  - `ai-gateway/kinds/` (routes, declared checks, role) and the regenerated manifest and types;
  - `pkg/internal/auth` (`catalog` access);
  - `pkg/internal/gateway` (route table, parent check);
  - `pkg/internal/service` (selector branch, stack catalog lister);
  - `pkg/internal/discovery`, `internal/host` and `providerwire/v4` (host category);
  - `test/platformhost` (subresource routes with a parent).
- **APIs:**
  - two new routes and one new role;
  - the discovery document and ProviderWire requests and responses are unchanged;
  - one new host error, which uses the existing 424 document.
- **Tests:**
  - unit tests for access, the parent check, the lister and the route table;
  - plugin-protocol tests through the test host;
  - cross-language scenarios in `ai-gateway/test/providerwire-v4`;
  - Playwright specs under `ai-gateway/tests/`, with a fake native provider reached through a TLS-intercepting proxy, both used only in end-to-end tests.
- **Not parity-sensitive:** the `/config` document and the ProviderWire wire format stay the same as on the namespace routes; `test/conformance/PARITY.md` maps the Gateway to its `gateway-*` specs. Grafana's own refusals (`403`, `404`, decrypt failures) are Kubernetes `Status` JSON, like today's platform errors.
