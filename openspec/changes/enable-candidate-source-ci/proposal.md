## Why

The #249 foundation supplies an explicit candidate Gateway workspace, but required CI still compiles published modules against older pinned dependencies. A coherent SDK/provider/middleware/Gateway source PR therefore cannot merge until prerequisite modules have been published. Issue #258 activates integrated source validation before #21 while retaining independent, revision-specific publication safety.

## What Changes

- Make required source checks run candidate SDK modules via root `go.work` and Gateway via explicit `go.gateway.work`, including hidden testserver/probe paths; keep formatting, security, parity, conformance, docs, integration, ancestry, policy fixtures and module/license boundaries blocking.
- Keep public-proxy standalone all-module and selected-module commands; report failing standalone PR diagnostics without making them (or a dependent aggregate) merge requirements. Keep declared/selected merged-pin provenance and reverse-dependency checks blocking.
- Gate Gateway image publication and deployment on standalone, readonly, replacement-free public-proxy validation and production image validation of the **same checkout SHA**; a push/tag alone never authorizes an image. Leave SDK tagging/manual module publication outside automation; maintainers validate selected modules standalone before manually publishing.
- Update policy documentation and OpenSpec contracts, test failure paths, and require an approved required-check/ruleset migration before rollout. #21 must incorporate #245 release readiness before its release automation can run with relaxed source gates.

## Capabilities

### New Capabilities
- `candidate-source-ci`: Required source eligibility, separate artifact-revision gating, diagnostic isolation and rollout safety for coordinated changes.

### Modified Capabilities
- `module-validation-modes`: Activate candidate Gateway mode for required source checks, preserve explicit standalone modes and change isolation to use candidate source without Gateway.
- `merged-internal-pins`: Preserve blocking canonical ancestry while separating it from standalone compilation in source CI.
- `upstream-parity-governance`: Replace the older published-producer-before-consumer PR prerequisite with independent candidate-source mergeability and separate publication evidence; retain parity-check rigor.

## Impact

Planning targets `.github/workflows/ci.yml`, `mise.toml`, `scripts/module-policy.sh`, Gateway ProviderWire/client test build paths, workflow/module-policy tests, `AGENTS.md`, `CONTRIBUTING.md`, and the three affected specifications. Root `go.work` remains Gateway-free; `go.gateway.work` remains explicit; `ai-gateway/Dockerfile` remains standalone. No runtime, upstream baseline, release-please, repository settings, or tagging changes are included in this proposal.
