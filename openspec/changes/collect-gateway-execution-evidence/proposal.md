## Why

Gateway attribution and diagnostic protection need a focused review before they are connected to public runtime carriers. This second part of #322 extracts the bounded internal collector, projection and namespace schema from the approved implementation; it does not activate Gateway output.

## What Changes

- Add request-local candidate/decision/part observation, sealing and immutable bounded snapshots.
- Normalize candidate-local native errors with standard Go JSON, known-source protection, numeric precision and complete-component/aggregate budgets; no token-level JSON transformer.
- Define and validate the Gateway-owned namespace schema with focused package and schema tests.
- Separate execution facts from metadata assembly; remove repeated whole-state encoding, retention rescans and jsontext. Gateway/workspace and independent client minimums remain Go 1.26.3.
- With owner approval, use standard duplicate-member processing and string/key normalization for rewritten native diagnostics instead of the earlier lexical-preservation requirement. Opaque metadata and exact client HTTP/SSE retention are unchanged.

## Capabilities

### New Capabilities

- `gateway-evidence-collection`: Internal request-local attribution, protected bounded snapshots and their schema, without runtime activation.

### Modified Capabilities

None. Existing ProviderWire and fallback behavior remain unchanged until the integration change.

## Impact

Adds ai-gateway/internal/evidence, the namespace schema and schema tests; keeps the existing Go baseline unchanged. Stacked above retain-grafana-error-extensions and #332. The separate expose-gateway-attempts-and-failures change owns service/handler wiring, public emission, cross-client/frontend proof and consumer documentation. Native transport debugging (#323) and producer fixes (#299) remain outside this change.
