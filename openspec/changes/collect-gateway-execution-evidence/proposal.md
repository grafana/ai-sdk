## Why

Gateway attribution and diagnostic protection need a focused review before they are connected to public runtime carriers. This second part of #322 extracts the bounded internal collector, projection and namespace schema from the approved implementation; it does not activate Gateway output.

## What Changes

- Add request-local candidate/decision/part observation, sealing and immutable bounded snapshots.
- Project candidate-local native errors with known-source protection, raw JSON fidelity and complete-component/aggregate budgets.
- Define and validate the Gateway-owned namespace schema with focused package and schema tests.
- **BREAKING** for Gateway source builds: raise only the Gateway module/workspace minimum to Go 1.27 for standard-library jsontext. The independent client remains Go 1.26.3.

## Capabilities

### New Capabilities

- `gateway-evidence-collection`: Internal request-local attribution, protected bounded snapshots and their schema, without runtime activation.

### Modified Capabilities

None. Existing ProviderWire and fallback behavior remain unchanged until the integration change.

## Impact

Adds ai-gateway/internal/evidence, the namespace schema and schema tests; changes the Gateway Go baseline only. Stacked above retain-grafana-error-extensions and #332. The separate expose-gateway-attempts-and-failures change owns service/handler wiring, public emission, cross-client/frontend proof and consumer documentation. Native transport debugging (#323) and producer fixes (#299) remain outside this change.
