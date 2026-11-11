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
4. Restart Grafana and enable **Grafana AI Gateway** under
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
keys, `GRAFANA_JWKS_URL` or `GRAFANA_JWKS`, to verify callers. Without them,
it starts idle: it ignores its settings, serves nothing, and Grafana's log
warns that plugin requests will be rejected. A Grafana Cloud stack's own Grafana
runs the Gateway this way, because the platform serves the Gateway API
elsewhere.

## Choose who can use it

`access_mode` decides who can use the configured models:

- **`self-hosted`:** every user and service account that can use the app gets
  the configured models. Requests that bring their own provider keys are
  refused. To limit access, restrict the app with role-based access control,
  or give access only to service accounts.
- **`cloud`:** for Grafana Cloud. Only the Grafana services listed in
  `access_internal_services` get the configured models. Every other user and
  service account brings its own provider key with each request.

## Store model catalogs in Grafana

A stack can also store its own model catalogs in Grafana as `ModelCatalog`
resources, with each provider's API key held as a Grafana secure value, and
call their models through the catalog's own routes.

Admins manage catalogs on the app's **Model catalogs** page. A key you enter
there is sent once and never shown again: the page only shows that a provider
has one, and an edit that doesn't replace a key keeps the stored one. When
Grafana or the Gateway refuses a change, the page shows the message it returns.
Viewers and Editors see catalogs read-only. The page follows each user's Grafana permissions, so
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

- `spec` uses the catalog format above without `apiKeyEnv`. Providers must be
  `anthropic` or `openai`, named with lowercase letters, digits and `-`, and
  can't set `baseURL`, `providerName` or `supportsStructuredOutputs`.
- Each provider needs exactly one secure value with the provider's name. Reads
  return secret names, never keys. Updates can omit `secure` to keep the stored
  keys, and `remove` a key only together with its provider.
- Catalogs aren't in folders. Viewers and Editors can read them (`aigateway:catalogreader`)
  and Admins can write them (`aigateway:catalogwriter`).
- The Gateway validates every create and update, so writes fail while the
  plugin is unavailable. Deletes always work.

### Call a stored catalog

Use the catalog's URL as the client's base URL, for example
`https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123/modelcatalogs/default`.
Clients append `/config` and `/language-model` as they do for the Gateway URL,
so they need no other change.

- Editors and Admins can list and call catalog models (`aigateway:modelinvoker`).
  Viewers can read catalogs but not call them.
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
  catalog page shows changed permissions after a reload.
- Grafana itself answers `403` when the caller can't call the catalog and `404`
  when the catalog doesn't exist. These are Kubernetes `Status` documents, not
  Gateway error documents; both clients report them as errors with that status.
- A stored catalog that the Gateway can't use, such as one whose keys don't match
  its providers, gets a `424` `failed_dependency` error with a fixed message.
- This needs the Grafana version and feature toggles in
  [Requirements](#requirements). Grafana Cloud doesn't store secure values for
  plugin resources yet, so catalogs with keys are refused there.

## Settings

| Setting | Description |
|---|---|
| `model_catalog` | The model catalog YAML, usually read from a file with `$__file{}`. Optional: without it, the namespace routes list no configured models and refuse every model ID, while BYOK still works in `cloud` mode. |
| `access_mode` | `self-hosted` or `cloud`. Required. |
| `access_internal_services` | Comma-separated Grafana services that get the configured models in `cloud` mode. |
| `agento11y_*` | Optional export to Agent Observability; see [Monitor the Gateway](docs/text-observability.md#export-to-agent-observability). |

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
| 64 KiB model catalog; 1,024 model IDs and aliases in total, 128 aliases and 7 fallbacks per model; 128 characters per model ID and alias; 2,048 bytes per `name`, `description`, `providerName` and each `provider` and `model` value | Gateway, at startup |
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
