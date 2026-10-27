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
| `agento11y_*` | Optional export to Agent Observability; see [Monitor the Gateway](docs/text-observability.md#export-to-agent-observability). |

## Limits

Each limit has one owner. The Gateway keeps only the limits that protect
something it owns; Grafana owns the rest. Grafana rejects some requests, such
as those with an invalid token or above its size limit, before they reach the
Gateway; those errors use Grafana's error format.

Gateway limits:

| Limit | Owner |
|---|---|
| 8 provider attempts per request: a model's primary and fallbacks, or the keys for the selected provider in a request that brings its own keys | Model catalog loader, at startup, and request key selection |
| 16 MiB per provider response that is not a stream, counted after decompression; 10 seconds to receive response headers; no redirects | Model transport, shared by every provider and provider key |
| 30 seconds without stream activity; 1 second of cleanup after a stream ends | Language-model handler |
| 1 MiB model catalog file; 1,024 model IDs and aliases in total, 128 aliases and 7 fallbacks per model; 128 characters per model ID and alias; 2,048 bytes per `name`, `description`, `providerName` and each `provider` and `model` value | Model catalog loader, at startup |

Platform limits:

| Limit | Owner and source |
|---|---|
| Request duration, including a streamed response | Grafana's route deadline: 60 seconds today, configurable in a later Grafana release |
| Request size | The plugin route message size: 16 MiB, the Grafana plugin SDK default |
| Admission and how many requests run at once | Grafana |
| Process memory | The container memory limit. Set `GOMEMLIMIT` to about 85–90% of it. |

### Removed limits

| Removed | Replaced by |
|---|---|
| `max_request_bytes` setting | The plugin route message size |
| `max_model_duration` setting | Grafana's route deadline |
| 16 MiB per streamed provider response in total | The time limits |
| 16 candidates per model; 8 keys per request | 8 provider attempts per request |
| Sizes of a generated response, a stream event, the provider's raw usage, an error response and the provider error summaries; the number of stream events | The model transport limit and the time limits |

Both removed settings are ignored if they are still set.

### Request duration

Grafana's route deadline is the only limit on a whole request, including
selection, every provider attempt and a streamed response. When the deadline
expires, the Gateway sees the request as canceled, so its logs, metrics and
attempt records show a cancellation; Grafana's own request log shows the
deadline. Give the plugin a termination grace period longer than the route
deadline plus the 1 second of stream cleanup.

Without a route deadline, a request ends only when it completes, when the caller
disconnects, when a provider takes more than 10 seconds to send response
headers, or when a stream is idle for 30 seconds. A non-streaming Anthropic call
also fails before it reaches the provider when its token budget is larger than
the Anthropic SDK allows without a deadline: set `maxOutputTokens`, or stream.

### Memory

Memory per request grows with the request size, with the size of each provider
response or stream event, because encoding it as JSON can expand it several
times, and with the number of requests running at once. The Gateway does not
limit the size of a single stream event: the Anthropic and OpenAI SDKs limit a
line to 32 MiB, and the OpenAI-compatible provider has no limit yet
([#546](https://github.com/grafana/ai-sdk/issues/546)). Grafana, not the
Gateway, decides how many requests run at once, and the container memory limit
bounds the process. Setting `GOMEMLIMIT` below that limit makes the Go runtime
collect garbage before the container runs out of memory.

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
