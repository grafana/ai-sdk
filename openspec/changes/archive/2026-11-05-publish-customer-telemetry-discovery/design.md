# Design

## Context

See proposal.md. Cache and Pool are internal foundations with no production consumer. The accepted implementation plan uses stack head `23dc2c11`; later local experiments reproduced background disablement and stale-snapshot publication gaps.

## Goals / Non-Goals

Connect current validated metadata to typed pools without network work under publication locks. Do not introduce Cloud adapters, producers, global OTel providers, external activation or interpretation of opaque revisions.

## Decisions

- Serialize publication under the existing cache mutex, before releasing lookup waiters. Private synchronous hooks avoid an event queue, reordered snapshots and dropped revocations. Lock order is cache, pool, gate; no factory or shutdown callback runs under the cache lock.
- A controller owns a fixed set of typed deliveries. Construction claims empty, open pools; failed construction releases claims. Attached pools reject external Update calls. Acquisition waits for discovery, then selects current binding/policy under the owner lock, never publishing an earlier request snapshot. A lost acquisition race omits recording. The cache and pool retain all per-stack state; the typed bridge adds no duplicate map.
- Publish expiry even while refresh is pending or the entry is idle. Eviction revokes and forgets delivery metadata, including active leases, without changing inference. No heartbeat or polling keeps idle entries alive.
- Retiring gates only shorten validity when current authority shortens. A successor refresh cannot extend a retired target. Apply shortening before replacement capacity checks.
- Stop cache publication before concurrently closing cache and pools. Bound aggregate waiting by the largest configured pool shutdown budget and any earlier caller deadline. Repeated close never extends a pool's drain deadline.

### Upstream alignment

Registered baseline: ai 7.0.159, provider 4.0.35, gateway 4.0.132 at Vercel `028a025398782f4938303f48fbeee45c79af544b`. Read wrap-language-model.ts at this revision. Gateway tenant discovery/lifecycle is a Go host extension (PARITY.md and telemetry-integration-surface); no upstream algorithm is ported. Generate/stream order, provider requests, SSE and fixtures stay unchanged. No baseline upgrade.

## Risks / Trade-offs

- Lock inversion or blocking publication → fixed internal sinks only, factory/cleanup outside publication, race tests with blocked resolver/build.
- Recording outlives cache idle retention → eviction intentionally revokes it; document operational bound without extending it through request polling.
- Cross-signal sends are not atomic → guarantee denial of new admissions after publication completes, not recall of bytes already delivered.
- Source returns stale remote data as a new valid response → local serialization cannot detect that through opaque revisions; adapter consistency remains separate.
- Capture and metrics → this controller carries policy; it does not enforce producer allowlists, suppress marked generation spans when ineligible, or prove the SDK's metric profile.

## Migration Plan

Internal addition above the existing stack, with no production wiring. Revert this change without altering inference. Concrete adapters and recorder integration require separate validation.
