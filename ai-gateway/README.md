# Grafana AI Gateway

Grafana AI Gateway is a Grafana app that gives your applications one API for
models from Anthropic, OpenAI and OpenAI-compatible providers. You publish
models under stable names, with aliases and fallbacks, and keep provider keys
in Grafana's configuration. Applications call the Gateway through your Grafana
instance with a Grafana credential, using the
[Go client](../docs/providers/grafana-gateway.md) or the Vercel AI SDK Gateway
client.

## Requirements

- Grafana 13.3 or later. Calling a stored catalog also needs Grafana to run the
  access checks that the catalog routes declare; the end-to-end tests verify
  this on Grafana's nightly image.
- The `appplugins.loadAppManifest`, `appplugins.registerAPIServer` and
  `grafana.useRouterMiddleware` feature toggles. The last one is experimental.
- The plugin is published only to the dev plugin catalog for Grafana Cloud.
  For other Grafana instances, build it from this directory and allow it as an
  unsigned plugin.

## Install

1. From the repository root, build the plugin with
   `mise run build-ai-gateway-plugin`, then copy `ai-gateway/dist/` to
   `grafana-aigateway-app` in your Grafana plugins directory.
2. Add the plugin to your Grafana configuration:

   ```ini
   [feature_toggles]
   appplugins.loadAppManifest = true
   appplugins.registerAPIServer = true
   grafana.useRouterMiddleware = true

   [plugins]
   allow_loading_unsigned_plugins = grafana-aigateway-app

   [plugin.grafana-aigateway-app]
   model_catalog = $__file{/etc/grafana/aigateway/models.yaml}
   access_mode = self-hosted
   anthropic_api_key = $__file{/run/secrets/anthropic-api-key}
   ```

3. Create the model catalog file that `model_catalog` reads, as described in
   the next section.
4. Restart Grafana and enable **AI Gateway** under
   **Administration > Plugins and data > Plugins**.

The app's overview page shows the Gateway URL that applications use, and the
URL of each stored model catalog.

## Configure models

The model catalog is a YAML file with the provider accounts the Gateway can
use and the models it offers:

```yaml
providers:
  anthropic:
    type: anthropic
    apiKeyEnv: GF_PLUGIN_ANTHROPIC_API_KEY
  openai:
    type: openai
    apiKeyEnv: GF_PLUGIN_OPENAI_API_KEY
models:
  balanced:
    name: Balanced
    description: General-purpose model
    aliases: [default]
    primary:
      provider: anthropic
      model: claude-sonnet-4-6
    fallback:
      - provider: openai
        model: gpt-5
```

Each provider needs a `type` (`anthropic`, `openai` or `openai-compatible`)
and an `apiKeyEnv`. An `openai-compatible` provider also needs a `baseURL`;
the other types use it to replace the provider's default endpoint. Set
`supportsStructuredOutputs: true` on an `openai-compatible` provider to send
schema requests as JSON-schema response formats instead of JSON mode. It
defaults to `false`, and setting it to `true` on other types fails at startup.
Only enable it for backends you have verified support JSON-schema outputs.

Each model needs a `name` and a `primary` provider and model. Applications
select it by its ID, such as `balanced`, or by one of its `aliases`. When the
primary model fails before it starts answering, the Gateway tries the
`fallback` models in order.

### Allow Anthropic server-side fallbacks

Applications can ask Anthropic to retry a refused turn on another model with
`providerOptions.anthropic.fallbacks`. The Gateway refuses that request unless
the candidate that would send it allows it with `serverFallbacks`:

```yaml
    primary:
      provider: anthropic
      model: claude-opus-5
      serverFallbacks:
        models: [claude-opus-4-8]
        allowDefault: true
```

`models` lists the fallback models an explicit chain may name; each may appear
once in a request and must match exactly. `allowDefault: true` also allows
`"default"`, which lets Anthropic choose the model, and its cost. The policy
applies only to the `primary` or `fallback` entry that declares it, only
`anthropic` providers accept it, and it must allow `"default"` or at least one
model. Every entry that names the same provider and backend model must carry
the same policy, in any route, for the reason given for skills below. Requests
that bring their own provider keys are not limited by it.

### Allow Anthropic agent skills

Anthropic agent skills, such as `pptx`, are refused unless the primary or
fallback entry lists them under `skills.allow`. Only `anthropic` providers can
carry a skills policy:

```yaml
models:
  documents:
    name: Documents
    primary:
      provider: anthropic
      model: claude-sonnet-4-6
      skills:
        allow:
          - type: anthropic
            skillId: pptx
          - type: custom
            skillId: skill_01AbCdEf
            versions: ["1759178010641129"]
```

- An `anthropic` skill is provided by Anthropic. A `custom` skill lives in the
  provider account, and listing it shares it with every caller of that model;
  there is no per-tenant scoping.
- Without `versions`, only requests that omit the version are allowed. With
  `versions`, the request must send one of them; `latest` is matched literally.
- Every entry that names the same provider and backend model must carry the
  same policy, in any route, because a call that combines routes tries that
  backend model only once. To use different policies for one backend model,
  declare a second provider with the same `apiKeyEnv`.
- Calls on these models cannot send a `container.id`, with or without skills,
  because containers belong to the provider account. Each call starts a fresh
  container.

Each key of the `[plugin.grafana-aigateway-app]` section reaches the Gateway as
an environment variable named `GF_PLUGIN_<KEY>`, so `anthropic_api_key` becomes
`GF_PLUGIN_ANTHROPIC_API_KEY`. Keep provider keys there and read them from a
file with `$__file{}` or from Grafana's environment with `$__env{}`.

The Gateway reads the catalog from the `model_catalog` setting, never from a
file. `$__file{}` makes Grafana read the file when Grafana starts, so restart
Grafana after you change it. Hosts that configure plugins through the
environment set `GF_PLUGIN_MODEL_CATALOG` to the catalog YAML instead; if their
settings cannot contain newlines, pass the catalog as single-line JSON, which is
also valid YAML. If the file that `$__file{}` names is missing or unreadable,
Grafana itself fails to start.

The Gateway checks its settings and the catalog when Grafana starts it. If
something is wrong, it does not start, and Grafana's log says why.

The Gateway serves requests only when Grafana gives it the platform signing
keys, `GF_JWKS_URL` or `GF_JWKS`, to verify callers; older Grafana versions
name them `GRAFANA_JWKS_URL` and `GRAFANA_JWKS`, which it also reads. Without them,
it starts idle: it ignores its settings, serves nothing, and Grafana's log
warns that plugin requests will be rejected. A Grafana Cloud stack's own Grafana
runs the Gateway this way, because the platform serves the Gateway API
elsewhere.

## Choose who can use it

`access_mode` decides who can use the configured models:

- **`self-hosted`:** every user and service account that can use the app gets
  the configured models. Requests that bring their own provider keys are
  refused. Using the app takes the `plugins.app:access` permission on scope
  `plugins:id:grafana-aigateway-app`. Viewers have it for every app by
  default, through scope `plugins:*`, so to limit access, remove that
  permission from the basic roles and grant it on the Gateway's scope only to
  the roles that need it, as described in
  [Prevent Viewers from accessing an app plugin](https://grafana.com/docs/grafana/latest/administration/roles-and-permissions/access-control/plan-rbac-rollout-strategy/#prevent-viewers-from-accessing-an-app-plugin).
  Changing basic roles needs Grafana Enterprise. A plugin calling as itself,
  with no user, is refused unless it is listed in `access_internal_services`.
- **`cloud`:** for Grafana Cloud. Only the Grafana services listed in
  `access_internal_services` get the configured models. Every other user and
  service account brings its own provider key with each request.

Each `access_internal_services` entry is the service identity a caller
presents, the `serviceIdentity` claim of its access token, matched exactly. A
listed service gets the configured models in either mode, as itself and on
behalf of a user. In self-hosted Grafana, a plugin's service identity is
`plugin:<plugin ID>`, which needs a Grafana version that signs it
([grafana/grafana#139015](https://github.com/grafana/grafana/pull/139015)). List
only the plugins that may use the configured models without a user: a plugin
calling as itself has no Grafana role, so `plugins.app:access` doesn't limit it.

## Store model catalogs in Grafana

A stack can also store its own model catalogs in Grafana as `ModelCatalog`
resources, with each provider's API key held as a Grafana secure value, and
call their models through the catalog's own routes.

Admins manage catalogs on the app's **Model catalogs** page. Create a catalog
with a name and its first provider. Enter the provider type and API key, and
a [custom endpoint](#custom-endpoints) if needed. The app saves the catalog
without models, opens its page, and checks provider health once using the
saved key if you have permission. You can then connect more providers and add
models. You can remove the last model, but to remove the last provider you
must delete the catalog. Adding a model takes only the provider's model
ID, which applications then request it by; a model's details, such as another
ID, a display name, aliases and fallbacks, are edited from the model. Each
change is saved when you confirm it. A key you enter is sent once and never
shown again. An edit that doesn't replace a key keeps the stored one. When Grafana or the Gateway refuses
a change, the page shows the message it returns. Viewers and Editors see
catalogs read-only. The page follows each user's Grafana permissions, so
granting or removing `aigateway:catalogwriter` changes what it offers them.
You can also write catalogs through the API, for example:

```yaml
apiVersion: aigateway.ext.grafana.app/v1alpha1
kind: ModelCatalog
metadata:
  name: default
spec:
  providers:
    anthropic: { type: anthropic }
  models:
    balanced:
      name: Balanced
      primary: { provider: anthropic, model: claude-sonnet-4-6 }
secure:
  anthropic: { create: "<provider key>" }
```

Reads return `secure.anthropic.name: inline-<generated>` in place of the key.
Grafana creates that secret for the catalog and owns it there: replacing a key
deletes the old secret, and deleting the catalog deletes its secrets.

- `spec` uses the catalog format above without `apiKeyEnv`. Providers are
  named with lowercase letters, digits and `-`, and a custom `baseURL` must be
  [listed](#custom-endpoints).
- `spec.providers` must contain at least one provider. `spec.models` can be an
  empty map: discovery then returns `{"models":[]}`. Calls return the
  unknown-model error unless a provider serves unlisted models. The operator
  `model_catalog` setting still requires at least one provider and one model
  when set.
- Each provider needs exactly one secure value with the provider's name. Reads
  return secret names, never keys. Updates can omit `secure` to keep the stored
  keys, and `remove` a key only together with its provider.
- Catalogs aren't in folders. Viewers and Editors can read them (`aigateway:catalogreader`)
  and Admins can write them (`aigateway:catalogwriter`).
- The Gateway validates every create and update, so writes fail while the
  plugin is unavailable. Deletes always work.

### Serve every model of a provider

Providers connected in the app start with **Serve every model** on, including
the first provider in a new catalog. You can change the mode on the provider
card. This does not change existing providers: an omitted `serveAllModels`
field still means listed models only. In **Serve every model** mode, the picker
switches to **Blocked model IDs**; adding or removing an ID saves immediately.
To set exclusions before enabling, select **Edit blocked IDs** while the
switch is off. Existing listed models stay visible and editable. With the API,
set `serveAllModels: true` and optionally `modelBlocklist: ["model-to-block"]`
on that provider in `spec.providers`.
The blocklist applies to language-model routing and matches exact,
case-sensitive backend IDs on that provider instance, not prefixes or other
accounts. It does not cover provider-hosted tool options that name their own
model. The blocklist stays saved when you turn unlisted serving off. A listed
model's primary or fallback cannot use a blocked target. Removing a listed
model does not block its backend ID; add the ID to the blocklist to stop
serving it through that instance.

Call an unlisted model as `<provider-instance>/<model-id>`, for example
`team-openai/gpt-5` for the instance named `team-openai`. A listed ID or alias
wins if it matches the full selector. If no listed ID or alias matches, a
blocked selector returns `404 model_not_found`. Unlisted models have no
implicit fallback, but can use or be named in explicit `gateway.models`
fallbacks. `/config` still discovers only listed models. The provider still
decides which requests and models its account supports. The provider model ID
picker in either mode fetches suggestions separately, on demand.

### Custom endpoints

Stored catalogs, and in `cloud` mode the accounts a caller brings, can set a
`baseURL` other than the provider's own endpoint. The Gateway accepts only the
endpoints the operator lists:

```ini
[plugin.grafana-aigateway-app]
custom_endpoints = https://llm.corp.example/v1, https://eu.anthropic-proxy.example
```

- An endpoint matches an entry with the same scheme, host and port, and the
  same path or one below it: `https://llm.corp.example/v1/team` matches the
  first entry; `.../v10`, `http://...` and `...:8443/v1` don't. There are no
  wildcards, and `http` works only where listed.
- Paths with `.` or `..` segments, a backslash, a `;`, or an encoded `/`, `\`,
  `%` or `;` are refused.
- The provider's own endpoint needs no entry. Without the setting, only those
  endpoints work.
- The list is checked when a catalog is saved (`400`), on every call to a
  stored catalog (`424`) and on the accounts a call brings (`400`), without
  resolving or connecting to anything. Errors don't name the endpoint.

List only endpoints you control, or vendors whose DNS you trust: whoever
controls a listed host's DNS controls where requests and keys go. In `cloud`
mode, any user or service account can use a listed endpoint.

### Call a stored catalog

With a service account or caller-managed access token, use the catalog URL,
for example
`https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123/modelcatalogs/default`.

- ProviderWire clients use the catalog URL as the base URL and append
  `/config` or `/language-model`.
- With Go ProviderWire token exchange, keep the Gateway API root and select the
  catalog with `WithCatalog`. See the
  [authentication guide](../docs/guides/gateway-authentication.md#call-a-stored-stack-catalog).
- For an [OpenAI client](../docs/guides/openai-chat-completions.md), append
  `/v1` to the catalog base URL so the SDK calls
  `POST .../modelcatalogs/{name}/v1/chat/completions`.

- Editors and Admins can list and call catalog models (`aigateway:modelinvoker`).
  Viewers can read catalogs but not call them, and anonymous access can't
  call them even when Grafana's roles would allow it.
- A Grafana Cloud access policy token needs the
  `aigateway.ext.grafana.app/modelcatalogs:get` permission. `access_mode` and
  `access_internal_services` don't apply to catalog routes.
- Calls use only the catalog's models and keys, never the `model_catalog`
  setting or server keys, and refuse `providerOptions.gateway.byok`. Routing
  controls (`models`, `order`, `only`) and fallbacks work as for the
  configured models.
- An edit applies on the next call. A key rotated outside the catalog, without
  changing the resource, can take up to a minute to apply, because Grafana
  caches decrypted keys for that long.
- Removing a role, or revoking a token or access policy, can take longer to
  apply, because Grafana and Grafana Cloud cache permissions and tokens. The
  catalog page picks up changed permissions on a later reload, once Grafana's
  permission cache refreshes, and Grafana refuses writes the user can no longer
  make.
- Grafana itself answers `403` when the caller can't call the catalog and `404`
  when the catalog doesn't exist. These are Kubernetes `Status` documents, not
  Gateway error documents; both clients report them as errors with that status.
- A stored catalog that the Gateway can't use, such as one whose keys don't match
  its providers, gets a `424` `failed_dependency` error with a fixed message.
- This needs the Grafana version and feature toggles in
  [Requirements](#requirements). Grafana Cloud doesn't store secure values for
  plugin resources yet, so catalogs with keys are refused there.
- Catalog models can't allow [agent skills](#allow-anthropic-agent-skills)
  or [server-side fallbacks](#allow-anthropic-server-side-fallbacks) yet, so
  they refuse both ([#781](https://github.com/grafana/ai-sdk/issues/781)).

## Settings

| Setting | Description |
|---|---|
| `model_catalog` | The model catalog YAML, usually read from a file with `$__file{}`. Optional: without it, the namespace routes list no configured models and refuse every model ID, while BYOK still works in `cloud` mode. |
| `access_mode` | `self-hosted` or `cloud`. Required. |
| `access_internal_services` | Comma-separated service identities of the Grafana services and plugins that get the configured models, in either mode; see [Choose who can use it](#choose-who-can-use-it). |
| `custom_endpoints` | Comma-separated base URLs that catalogs and BYOK accounts may use; see [Custom endpoints](#custom-endpoints). |
| `agento11y_*` | Optional export to Agent Observability; see [Monitor the Gateway](docs/text-observability.md#export-to-agent-observability). |
| `extensions` | Optional YAML configuration of a distribution's operator extensions; see [Extend Grafana AI Gateway](../docs/guides/gateway-extensions.md). |

## Limits

Grafana rejects some requests before they reach the Gateway, such as those with
an invalid token or a body that is too large. Those errors use Grafana's error
format, not the Gateway's.

| Limit | Set by |
|---|---|
| Request duration, including a streamed response: 60 seconds | Grafana's plugin route timeout |
| Request size: 16 MiB | Grafana's plugin message size |
| Requests running at once | Grafana |
| Process memory | The container memory limit |
| 8 provider attempts per request: a model's primary and fallbacks, or the provider keys a request brings for the selected provider | Gateway |
| 16 MiB per provider response that is not streamed, after decompression | Gateway |
| 10 seconds for a provider to send response headers; provider redirects are not followed | Gateway |
| 30 seconds without activity in a stream | Gateway |
| 64 KiB model catalog; 1,024 model IDs and aliases in total, 128 aliases and 7 fallbacks per model; 128 characters per model ID and alias; 2,048 bytes per `name`, `description`, `providerName`, each `provider` and `model` value and each `serverFallbacks.models` entry | Gateway, at startup |
| The same model limits for `ModelCatalog` resources | Gateway, when a catalog is saved and each time a call uses it |

### Request duration

A request ends at Grafana's route timeout, including every fallback attempt and
a streamed response. The Gateway records a request that reaches the timeout as
canceled in its logs and metrics. Set the plugin's termination grace period
longer than the route timeout, so that requests in progress can finish.

### Memory

Each request holds its body and the provider response in memory, one event at a
time for a streamed response. Encoding a response as JSON can need several times
its size. Total memory grows with the number of requests running at once, which
Grafana controls. Set `GOMEMLIMIT` to about 85–90% of the container memory
limit, so that the Go runtime frees memory before the container runs out.

## Learn more

- [Gateway architecture](../docs/concepts/gateway-architecture.md) and
  [glossary](../docs/concepts/gateway-glossary.md): the request path and
  component boundaries.
- [Authenticate to the Gateway](../docs/guides/gateway-authentication.md):
  credentials for your applications.
- [Call the Gateway from Go](../docs/providers/grafana-gateway.md).
- [Monitor the Gateway](docs/text-observability.md): logs, metrics, traces and
  Agent Observability.
- [Sources](docs/sources.md): citations in model responses.
- [Model catalog package](docs/model-catalog.md): build your own model catalog
  in Go.

## Develop locally

From the repository root, build the plugin and start Grafana on
<http://localhost:3000> with a development catalog:

```bash
mise run build-ai-gateway-plugin
export ANTHROPIC_API_KEY=...        # optional; needed only to call models
pnpm --filter ./ai-gateway server
```

Grafana runs the nightly image that the end-to-end tests use, pinned by digest
in `docker-compose.yaml`, because the plugin needs features that only recent
nightlies have. Set `GRAFANA_VERSION` to another tag to try it.

Any Anthropic API key works. Create one with your own Anthropic account or
organization; this repository does not provide one. The automated tests use a
deterministic stand-in for Anthropic and need no key.

Then work from `ai-gateway/`:

| After changing | Run |
| --- | --- |
| The frontend | `pnpm dev`, and keep it running. The page reloads on each change. |
| The backend in `pkg/` | `mage -v build:linux` (`build:linuxARM64` on arm64), then `docker compose exec grafana pkill -f gpx_aigateway`. Grafana restarts the plugin. |
| `provisioning/grafana.ini` or `provisioning/aigateway/models.yaml` | `docker compose restart grafana` |

The Gateway refuses Grafana's anonymous sign-in. Create a service account token
to call it:

```bash
grafana=http://localhost:3000
routes=$grafana/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/default
name=gateway-dev-$(date +%s)
id=$(curl -fsS -H 'Content-Type: application/json' \
  -d "{\"name\":\"$name\",\"role\":\"Viewer\"}" $grafana/api/serviceaccounts | jq .id)
token=$(curl -fsS -H 'Content-Type: application/json' \
  -d "{\"name\":\"$name\"}" $grafana/api/serviceaccounts/$id/tokens | jq -r .key)
curl -H "Authorization: Bearer $token" $routes/config
```

`docker compose logs -f grafana` shows the logs, and
`docker compose down --volumes` stops Grafana and discards its data.

To test your changes:

```bash
mise run test-ai-gateway             # Go tests
mise run test-ai-gateway-frontend    # frontend checks
mise run test-ai-gateway-command     # client compatibility tests
mise run test-ai-gateway-e2e         # browser tests against Grafana
mise run validate-ai-gateway-plugin  # plugin package checks
```

See [Contributing](../CONTRIBUTING.md) for the rest of the development guide.

## License

Files in this directory are licensed under [AGPL-3.0-only](LICENSE). The SDK
is licensed under [Apache-2.0](../LICENSE).
