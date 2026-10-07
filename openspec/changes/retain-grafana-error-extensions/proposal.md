## Why

The independent Grafana client currently discards additive Gateway failure data, preventing callers from inspecting diagnostics already present in a valid response. This is the client-only first part of #322 under the approved #321 contract, stacked on #332.

## What Changes

- Retain complete bounded valid HTTP error envelopes in the existing API-call cause's Data and ResponseBody.
- Retain exact committed SSE error.data in Data, leaving ResponseBody empty instead of manufacturing an HTTP response.
- Simplify exact-name decoding with private named DTOs while preserving classification, status-derived retryability, bounds and ordered consumption.
- Add focused regression tests and client guidance without implementing Gateway evidence production.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `grafana-gateway-client`: Preserve opaque additive error extensions in existing error carriers.

## Impact

Only the independent Apache client, its tests and consumer documentation change. Its Go 1.26.3 baseline, public API and dependency boundary remain unchanged. Gateway collection/schema and ProviderWire integration have separate changes, `collect-gateway-execution-evidence` and `expose-gateway-attempts-and-failures`.
