# Grafana AI Gateway

Grafana AI Gateway is a Grafana app that gives your applications one API for
models from Anthropic, OpenAI and OpenAI-compatible providers. You publish
models under stable names, with aliases and fallbacks, and keep provider keys
in Grafana's configuration. Applications call the Gateway through your Grafana
instance with a Grafana credential, using the
[Go client](../docs/providers/grafana-gateway.md) or the Vercel AI SDK Gateway
client.

## Requirements

- Grafana 13.3 or later.
- The `appplugins.loadAppManifest`, `appplugins.registerAPIServer` and
  `grafana.useRouterMiddleware` feature toggles. The last one is experimental.
- The plugin is not in the plugin catalog yet. Build it from this directory
  and allow it as an unsigned plugin.

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
   config_file = /etc/grafana/aigateway/models.yaml
   access_mode = self-hosted
   anthropic_api_key = $__file{/run/secrets/anthropic-api-key}
   ```

3. Create the model catalog in `config_file`, as described in the next section.
4. Restart Grafana and enable **Grafana AI Gateway** under
   **Administration > Plugins and data > Plugins**.

The app's overview page shows the Gateway URL that applications use.

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
the other types use it to replace the provider's default endpoint.

Each model needs a `name` and a `primary` provider and model. Applications
select it by its ID, such as `balanced`, or by one of its `aliases`. When the
primary model fails before it starts answering, the Gateway tries the
`fallback` models in order.

Each key of the `[plugin.grafana-aigateway-app]` section reaches the Gateway as
an environment variable named `GF_PLUGIN_<KEY>`, so `anthropic_api_key` becomes
`GF_PLUGIN_ANTHROPIC_API_KEY`. Keep provider keys there and read them from a
file with `$__file{}` or from Grafana's environment with `$__env{}`.

The Gateway checks its settings and the catalog when Grafana starts it. If
something is wrong, it does not start, and Grafana's log says why.

## Choose who can use it

`access_mode` decides who can use the configured models:

- **`self-hosted`:** every user and service account that can use the app gets
  the configured models. Requests that bring their own provider keys are
  refused. To limit access, restrict the app with role-based access control,
  or give access only to service accounts.
- **`cloud`:** for Grafana Cloud. Only the Grafana services listed in
  `access_internal_services` get the configured models. Every other user and
  service account brings its own provider key with each request.

## Settings

| Setting | Description |
|---|---|
| `config_file` | Path to the model catalog. Required. |
| `access_mode` | `self-hosted` or `cloud`. Required. |
| `access_internal_services` | Comma-separated Grafana services that get the configured models in `cloud` mode. |
| `max_request_bytes` | Largest request body. Defaults to 1 MiB; raise it for large file inputs. |
| `max_model_duration` | Longest model call. Defaults to `120s`; at least `30s`. |
| `agento11y_*` | Optional export to Agent Observability; see [Monitor the Gateway](docs/text-observability.md#export-to-agent-observability). |

## Limits

- Grafana ends a request after 60 seconds, including a streamed response.
- Grafana rejects some requests, such as those with an invalid token, before
  they reach the Gateway. Those errors use Grafana's error format.

## Learn more

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
| The backend or `provisioning/aigateway/models.yaml` | `GOWORK=$PWD/../go.gateway.work mage -v build:linux` (`build:linuxARM64` on arm64), then `docker compose exec grafana pkill -f gpx_aigateway`. Grafana restarts the plugin. |
| `provisioning/grafana.ini` | `docker compose restart grafana` |

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
