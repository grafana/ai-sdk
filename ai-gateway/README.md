# Grafana AI Gateway

Grafana AI Gateway is the server product that exposes compatible model APIs,
routes requests through a public model catalog, and applies authentication,
policy, observability, and fallback. It is developed in this repository as the
separate Go module `github.com/grafana/ai-sdk/ai-gateway`.

## Current status

This directory currently contains the ProviderWire V4 request contract and its
exact-pinned registered-client evidence. It does not yet provide an executable
Gateway server or Go client.

Gateway-owned public API adapters, protocol schemas and DTOs, authentication,
host policy, catalog, service composition, images, and deployment assets belong
under this directory. Reusable provider contracts, providers, middleware,
fallback primitives, and independent clients remain in the Apache-licensed SDK
outside this directory.

## License boundary

Unless a nearer license states otherwise, files under `ai-gateway/` are licensed
under [AGPL-3.0-only](LICENSE). Files outside `ai-gateway/` remain licensed under
the repository's [Apache License 2.0](../LICENSE).

The dependency boundary is one-way: AI Gateway may import explicitly pinned SDK
modules, but SDK modules must not import or require the AI Gateway module. Using
the Gateway over HTTP does not require importing Grafana AI SDK.

This repository boundary does not revoke or alter licenses already granted for
published revisions.

## Legal readiness

Before this license-boundary change merges or any Gateway build is deployed,
Grafana legal must confirm the effective transition, copyright provenance,
Apache-derived attribution, and the network corresponding-source offer
mechanism. Deployment documentation and images must identify the exact source
revision and satisfy that approved mechanism.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution rules and
[NOTICE](NOTICE) for provenance and attribution.
