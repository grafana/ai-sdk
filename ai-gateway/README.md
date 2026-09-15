# Grafana AI Gateway

Grafana AI Gateway is a separate Go module that exposes compatible model APIs
with authentication, policy, observability, routing, and fallback. Its module
path is `github.com/grafana/ai-sdk/ai-gateway`.

This directory contains the ProviderWire V4 request contract, exact-pinned
registered-client evidence, public model catalog, unary and streaming text HTTP
runtimes, the authenticated Anthropic and OpenAI-compatible service under
`cmd/grafana-ai-gateway`, and its container packaging. The service supports
`X-Access-Token` JSON Web Token (JWT) authentication or caller identity supplied
by a trusted authenticating reverse proxy. Provider credentials come from server
configuration in both modes. The reverse-proxy mode requires a separate listener
for operational routes and deployment network controls that allow only the proxy
to reach the API listener. A reusable Go Gateway client remains future work.

Gateway code may import explicitly pinned SDK modules. SDK modules must not
import, require, or replace the Gateway module, which remains outside the root
`go.work`.

Run `mise run test-providerwire-v4` for the contract and
`mise run verify-ai-gateway-boundary` for the module boundary. Repository-wide
development guidance is in [`../CONTRIBUTING.md`](../CONTRIBUTING.md).

See the [model catalog guide](docs/model-catalog.md) for public model identity
and resolution behavior, the [Cloud authentication guide](docs/cloud-authentication.md)
for identity headers, proxy isolation, and supported client calls, and the
[container guide](../docs/guides/ai-gateway-container.md) for image builds,
runtime configuration, and publication status. See the
[text observability guide](docs/text-observability.md) for the logical telemetry
contract, privacy boundary, metrics, and exporter configuration.

Files under this directory are licensed under [AGPL-3.0-only](LICENSE). The
reusable SDK remains [Apache-2.0](../LICENSE).
