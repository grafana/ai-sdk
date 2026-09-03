# Grafana AI Gateway

Grafana AI Gateway is a separate Go module that exposes compatible model APIs
with authentication, policy, observability, routing, and fallback. Its module
path is `github.com/grafana/ai-sdk/ai-gateway`.

This directory contains the ProviderWire V4 request contract, exact-pinned
registered-client evidence, public model catalog, and unary text HTTP runtime.
Streaming, authentication, service composition, deployment assets, and the Go
Gateway client land in later work packages.

Gateway code may import explicitly pinned SDK modules. SDK modules must not
import, require, or replace the Gateway module, which remains outside the root
`go.work`.

Run `mise run test-providerwire-v4` for the contract and
`mise run verify-ai-gateway-boundary` for the module boundary. Repository-wide
development guidance is in [`../CONTRIBUTING.md`](../CONTRIBUTING.md).

See the [model catalog guide](docs/model-catalog.md) for public model identity
and resolution behavior.

Files under this directory are licensed under [AGPL-3.0-only](LICENSE). The
reusable SDK remains [Apache-2.0](../LICENSE).
