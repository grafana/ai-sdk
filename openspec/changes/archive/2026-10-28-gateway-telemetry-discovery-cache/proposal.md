# Proposal

## Why

The contracts in [#590](https://github.com/grafana/ai-sdk/pull/590) separate customer destinations, recording policy, and ingestion authority, but each consumer would still need to coordinate lookups and freshness. The next unblocked slice of [#588](https://github.com/grafana/ai-sdk/issues/588) adds bounded, request-time resolution without static customer enrollment or dependence on a deployed control-plane adapter.

## What Changes

- Add an internal cache that accepts authorized stack attribution and resolves destination and policy snapshots independently on demand.
- Deduplicate concurrent lookups, preserve each caller's cancellation, and bound metadata waiting without changing inference execution.
- Refresh active entries before expiry, retain only still-valid state after transient failure, and immediately apply observed disablement or removal.
- Cache negative outcomes briefly, limit retained and in-flight work, clean up idle entries, and cancel owned work during shutdown.
- Verify the behavior with deterministic clocks and fake resolvers. Keep production request handling and live discovery disconnected.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-customer-telemetry`: Add request-time caching, independent freshness, bounded waiting, failure classification, resource ownership, and shutdown requirements.

## Impact

Extend `ai-gateway/cmd/grafana-ai-gateway/internal/customertelemetry` with cache implementation and focused tests; update the Gateway architecture/glossary. No public SDK API, provider protocol, exporter, deployment flag, customer configuration API, or real control-plane field mapping is introduced.

This change is stacked on #590. It implements the cache slice from the agreed sequence while the verified Cloud adapter remains a separate prerequisite for live integration. Future PRs must supply measured deployment settings and prove actual lookup and ingestion behavior; local fake-resolver tests establish only cache semantics.
