# Grafana AI Gateway

Grafana AI Gateway is a separate Go module that exposes compatible model APIs
with authentication, policy, observability, routing, and fallback. Its module
path is `github.com/grafana/ai-sdk/ai-gateway`.

This directory contains the ProviderWire V4 request contract, exact-pinned
registered-client evidence, public model catalog, unary and streaming text HTTP
runtimes, the authenticated Anthropic, OpenAI Responses and OpenAI-compatible
service under `cmd/grafana-ai-gateway`, and its container packaging. The service supports
private JWT/configured-account and trusted-Cloud/request-only BYOK access in one
process, with a third operational listener. Cloud application access must be
restricted to the authenticating proxy, including denial to internal JWT clients.
The Apache-licensed [Go client](../docs/providers/grafana-gateway.md) and pinned
Vercel Gateway client support both credential flows.

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
