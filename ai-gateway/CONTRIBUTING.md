# Contributing to Grafana AI Gateway

The repository-wide development, testing, parity, and OpenSpec workflow in
[`../CONTRIBUTING.md`](../CONTRIBUTING.md) applies here. This file records the
additional Gateway ownership and license rules.

## Contribution scope

Unless a nearer license states otherwise, contributions under `ai-gateway/` are
licensed under [AGPL-3.0-only](LICENSE). Contributions outside this directory
remain under the repository's [Apache License 2.0](../LICENSE). This scope does
not revoke or alter licenses granted for earlier published revisions.

Before this boundary change merges or any Gateway build is deployed, Grafana
legal must confirm the effective license transition, copyright provenance,
Apache-derived attribution, and network corresponding-source offer mechanism.
Contributors must not represent that confirmation as complete until Grafana has
recorded it through its approved process.

## Architecture boundary

Gateway-owned server code, public API adapters, protocol schemas and DTOs,
authentication, host policy, public catalog, service composition, configuration,
images, deployment assets, and implementation-owned tests belong here.

Reusable provider-domain contracts, providers, middleware, fallback primitives,
and independent clients belong outside `ai-gateway/`. Gateway code may import
explicitly pinned SDK modules. Code outside this directory must not import or
require `github.com/grafana/ai-sdk/ai-gateway`.

Run the focused boundary check before submitting changes:

```bash
mise run verify-ai-gateway-boundary
```

ProviderWire contract changes must also run:

```bash
mise run test-providerwire-v4
mise run validate-parity-baseline
```
