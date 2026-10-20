# Grafana AI Gateway

Grafana AI Gateway is a Grafana app plugin, `grafana-aigateway-app`, that serves
compatible model APIs with access policy, observability, routing and fallback.
It is built from this directory, whose Go module path is
`github.com/grafana/ai-sdk/ai-gateway`.

The Gateway serves the [ProviderWire V4](providerwire/v4) protocol that the
pinned Vercel Gateway client and the Apache-licensed
[Go client](../docs/providers/grafana-gateway.md) speak. It forwards calls to
Anthropic, OpenAI Responses and OpenAI-compatible providers.

## Hosting and routes

A Grafana App Platform plugin host runs the plugin backend: Grafana itself, or
the App Platform router in Grafana Cloud. The app manifest declares two
namespaced routes and no kinds:

| Method | Route |
|---|---|
| `GET` | `/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/{namespace}/config` |
| `POST` | `/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/{namespace}/language-model` |

Clients use `/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/<namespace>` as
their base URL. The host authenticates every caller and exchanges the caller's
credential for a token delegated to the plugin. The Gateway verifies that
token's signature, audience and namespace before it does any work. The plugin
opens no listeners and serves no health or metrics routes; health and metrics
go through the plugin protocol.

## Access modes

`access_mode` is required and has no default. The Gateway
reads the verified token's actor chain. The direct caller is the service that
called the host, and the identity actor is the user or service account the call
is for, when there is one.

| Mode | Caller | Access |
|---|---|---|
| `cloud` | Direct caller on the internal service allowlist | Configured models and discovery, also on behalf of a user |
| `cloud` | Any other user or service account | Request-scoped BYOK only |
| `self-hosted` | User or service account | Configured models and discovery; BYOK is refused |
| either | Anything else | Refused with the permission error |

In `cloud` mode, `access_internal_services` lists the signed
service identities allowed to use configured models, comma-separated and
matched exactly; whitespace around entries is ignored. A service that relays another service's token is the direct
caller, so it does not inherit that service's access. BYOK is open to every
stack user and service account; their provider keys are used only for their
own requests. The allowlist is rejected in `self-hosted` mode.

Grafana admits callers to the routes by plugin app access; the plugin declares
no roles of its own, because Grafana does not yet apply manifest roles to
custom routes. In Cloud the router grants app access to every authenticated
caller.

In `self-hosted` mode, anyone Grafana admits to the plugin can use the
configured models. Plugin app access is part of Grafana's basic roles, so
restrict it with role-based access control where available, or give access
only to service accounts. Grafana tokens always belong to service accounts,
which have no owner: create one service account per application or person you
want to attribute usage to.

### Self-hosted Grafana today

Self-hosted Grafana serves App Platform plugin routes only with the experimental
`grafana.useRouterMiddleware` feature toggle, together with
`appplugins.loadAppManifest` and `appplugins.registerAPIServer`. With those
toggles and no token exchange configured, Grafana 13.3 signs the tokens it
sends to the plugin itself and passes the plugin their signing keys in
`GRAFANA_JWKS`, so the caller's identity reaches the Gateway with no extra
service. A Grafana configured with `[plugins] v3_token_exchange_url` uses that
exchange instead; then set its signing keys in `GRAFANA_JWKS_URL`.

## Configuration

Settings go in Grafana's `[plugin.grafana-aigateway-app]` configuration
section, and the model catalog in a YAML file that names provider secrets by
environment variable:

```ini
[plugin.grafana-aigateway-app]
config_file = /etc/grafana/aigateway/models.yaml
access_mode = self-hosted
```

```yaml
providers:
  anthropic-primary:
    type: anthropic
    apiKeyEnv: ANTHROPIC_API_KEY
models:
  grafana/assistant:
    name: Grafana Assistant
    primary:
      provider: anthropic-primary
      model: claude-sonnet-4-6
```

| Setting | Purpose |
|---|---|
| `config_file` | Model catalog file. Required. |
| `access_mode` | `cloud` or `self-hosted`. Required. |
| `access_internal_services` | Internal service allowlist, `cloud` mode only. |
| `max_request_bytes` | Largest accepted request body, 1 MiB by default. Raise it for large file inputs. |
| `max_model_duration` | Longest model call, `120s` by default and at least `30s`. |
| `agento11y_*` | Optional Agent Observability export; see [text observability](docs/text-observability.md#agent-observability-configuration). |

Grafana passes each key to the plugin as `GF_PLUGIN_<KEY>`, for example
`GF_PLUGIN_ACCESS_MODE`. A host that starts the plugin itself, such as the App
Platform router, sets those variables directly. Token signing keys come from
app-sdk's `GRAFANA_JWKS_URL` or `GRAFANA_JWKS`, and exactly one is required;
self-hosted Grafana sets `GRAFANA_JWKS` itself.
Other limits, such as response sizes and stream timeouts, are fixed.

Invalid settings, or an `auth` or `server` block in the catalog file stop the
plugin before it serves requests. The Gateway always verifies token
signatures; app-sdk's `insecure_skip_authentication` has no effect.

Provider keys named in the catalog, and the Agent Observability secret, are
read from the plugin's environment. Grafana forwards its own environment only
to plugins listed in `[plugins] forward_host_env_vars`, so list
`grafana-aigateway-app` there when the keys are set on Grafana's process. The
App Platform router passes the environment of its plugin deployment.

## Lifecycle and limits

The host drains in-flight requests when it stops the plugin, so its termination
grace period must exceed `max_model_duration`
(120 seconds by default) plus the stream drain time. After the host stops the
plugin, the Gateway flushes Agent Observability and attempt records within one
second. The host's tracer shutdown shares the same window before the host kills
the process, so a slow trace exporter can cut the flush short.

Current platform limits:

- Platform routes end after 60 seconds, which also ends longer streams.
- Errors the platform produces before the Gateway runs, such as failed
  authentication or an exceeded deadline, use Kubernetes `Status` JSON rather
  than ProviderWire error documents.

## Build and validate

```bash
mise run test-ai-gateway-frontend    # frontend typecheck, lint and unit tests
mise run build-ai-gateway-plugin     # frontend, Linux amd64/arm64 backends, licenses
mise run validate-ai-gateway-plugin  # package contents and the Grafana plugin validator
mise run test-ai-gateway             # Go tests, including the built plugin process
mise run test-ai-gateway-command     # cross-language suites through the plugin test host
mise run test-ai-gateway-e2e         # Playwright tests against the built plugin in Grafana
```

The package includes the Gateway and SDK licenses, every linked Go module with
its license files per platform, bundled frontend licenses and the source
revision. Local builds are marked as unverified development builds. CI keeps the
validated package of eligible pushes as a workflow artifact; the plugin is not
published yet.

The plugin follows the create-plugin scaffold with its App Platform addition:
`.config/` is scaffold-owned and changes only through `create-plugin update`,
and `kinds/` holds the manifest source for `generate:kinds`.

For a local Grafana, build the plugin and run `pnpm --filter ./ai-gateway server`.
It starts the create-plugin compose environment on Grafana nightly with the App
Platform feature toggles, `self-hosted` access with Grafana's own token
signing, and the catalog in
`provisioning/aigateway/models.yaml`. Export `ANTHROPIC_API_KEY` first to call
models. The environment signs in anonymously, which the Gateway refuses, so call
the routes with a service account token. `pnpm --filter ./ai-gateway e2e` runs
the Playwright tests against a running environment; `mise run
test-ai-gateway-e2e` starts one, runs them and stops it. CI pins the Grafana
nightly image by digest.

## More

- [Model catalog](docs/model-catalog.md): public model identity and resolution.
- [Text observability](docs/text-observability.md): logs, metrics, traces and
  Agent Observability export.
- [Authenticate to the Gateway](../docs/guides/gateway-authentication.md):
  client credentials for both modes.
- [Contributing](../CONTRIBUTING.md): repository-wide development guidance.

Gateway code may import explicitly pinned SDK modules. SDK modules must not
import, require, or replace the Gateway module, which stays outside the root
`go.work`.

Files under this directory are licensed under [AGPL-3.0-only](LICENSE). The
reusable SDK remains [Apache-2.0](../LICENSE).
