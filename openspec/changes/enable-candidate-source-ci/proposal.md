## Why

Source PRs currently compile the Gateway against older published SDK/provider/middleware pins in required CI. A coordinated change cannot merge until its dependencies have been published, even when the candidate modules work together. Issue #258 separates source integration from artifact readiness without relaxing merged-pin provenance or the SDK-to-Gateway boundary.

## What Changes

- Run required SDK checks in root `go.work` and Gateway build/test/vet/lint and cross-language checks in explicit `go.gateway.work`, including the candidate Grafana client and copied semantic mutants.
- Keep merged-pin ancestry, structural boundary, source-absence, parity, formatting and integration checks blocking. Keep selected/all-module public-proxy standalone commands available, but not required for source PRs.
- Before publishing a Gateway image on an eligible main/tag push, make the existing image-validation job test the Gateway standalone with readonly public-proxy dependencies at the checkout revision, then build and smoke-test standalone images. Deployment follows successful publication only.
- Preserve required check names and document the separate #245/#21 SDK release-automation boundary. Do not change rulesets or enable SDK tagging here.

## Capabilities

### New Capabilities
- `candidate-source-ci`: Candidate-source merge eligibility and standalone Gateway artifact gating.

### Modified Capabilities
- `module-validation-modes`: Activate explicit Gateway candidate-source checks and use candidate root/client for Gateway-absent isolation while retaining standalone entry points.
- `merged-internal-pins`: Keep canonical ancestry blocking even when merged pins lag candidate source.
- `upstream-parity-governance`: Distinguish a green candidate-source PR from independently consumable published modules.

## Impact

Changes are limited to CI/task selection, the module-policy source-absence proof, the ProviderWire Go-client test build mode, workflow/policy tests and the related documentation. `go.work` remains Gateway-free; `ai-gateway/Dockerfile` remains standalone. No SDK runtime behavior or upstream baseline changes are intended.
