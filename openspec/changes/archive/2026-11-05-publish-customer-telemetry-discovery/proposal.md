# Proposal

## Why

The discovery cache and delivery pool have no shared owner. Background disablement cannot reach live exporters, and a consumer publishing an old request snapshot could restore revoked authority.

## What Changes

- Publish accepted discovery state, expiry and eviction through one ordered controller.
- Acquire typed delivery leases from current state without exposing request-owned publication.
- Bound retiring resources by shortened authority and compose cache/pool shutdown.
- Prove background revocation with deterministic tests and a real local OTLP transport.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-customer-telemetry`: ordered cache-to-pool publication, current-state acquisition, eviction and composed ownership.

## Impact

Internal customertelemetry package, its spec and architecture/glossary. No new dependencies, Cloud adapter, credential acquisition, generation producer, policy API or production activation. Capture allowlists, metric-profile enforcement, generation-marker eligibility and long-stream host acceptance remain separate integration work.
