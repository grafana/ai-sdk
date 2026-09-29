## Context

`module-resolution` currently combines public-proxy `verify-module-resolution` with canonical merged-pin and structural boundary checks. Gateway Go and ProviderWire testserver commands still default to `GOWORK=off`; the Grafana differential client also builds standalone, and the SDK/client source-absence proof compiles the client against an old root pin. `go.gateway.work` and selectable standalone validation already exist. This change switches required source execution without altering the production Dockerfile or registered upstream baseline.

## Goals / Non-Goals

**Goals:** independently green coordinated candidate-source PRs with older real merged pins; blocking ancestry, boundary, parity and integration; fail-closed standalone Gateway and image validation at the artifact revision.

**Non-Goals:** requiring per-PR standalone diagnostics, SDK tagging/release automation, runtime behavior or upstream baseline changes, repository settings changes.

## Decisions

1. **Required source paths select candidate modules.** Keep the existing required job names. Use root `go.work` for SDK/providers/middleware/examples and absolute `go.gateway.work` for Gateway build/test/vet/lint and its Go/TypeScript testservers. Keep the pinned TypeScript comparator, candidate Go-client differential and semantic copied-mutant red controls in blocking ProviderWire tests. A temporary workspace selects each copied mutant alongside candidate root, with source-selection assertions and semantic rather than setup/build failures. The SDK-only integration server stays Gateway-free. Conformance's nested module uses local replacements despite `GOWORK=off`; retain its behavior and parity coverage.

2. **Keep provenance and boundaries independent of compilation.** Remove all-module standalone execution from required `module-resolution`; retain `test-module-policy`, `verify-ai-gateway-boundary`, `verify-merged-pins`, candidate workspace selection and SDK/client Gateway-absent isolation. The latter runs copied candidate root and Grafana client in a Gateway-free root workspace, not against published root code. Selected and all-module standalone commands remain on-demand; no current-candidate standalone success is a prerequisite of any required source job.

3. **Reuse the existing artifact gate.** On canonical main or `ai-gateway/v*` pushes, `image-validation` verifies its checkout revision, then runs `MODULE=ai-gateway mise run verify-published-module` before any multiarch/native Docker build or smoke test. That command already enforces clean public-proxy cache, readonly module files, `GOWORK=off`, downloadability and no replacements. `publish-ai-gateway-image` keeps all required source jobs plus `image-validation` in `needs`; deployment requires successful publication. Skipped, cancelled or failed image validation cannot publish. The Dockerfile stays workspace-off and does not copy source workspaces. A source PR does not depend on image validation.

4. **Use bounded, honest proof.** Extend module-policy fixtures to demonstrate candidate module selection/execution with a stale declared pin, Gateway-absent isolation, unmerged/unverifiable ancestry and reverse-reference failures. Keep workflow tests as structural assertions about job/step order, dependencies, push guards and production workspace exclusion; these do not simulate GitHub Actions scheduling. Run the actual required source commands and ProviderWire semantic mutants locally. A full coordinated breaking-change workflow run with stale pins is rollout evidence, not an every-PR CI prerequisite.

## Rollout / Risks

Preserve existing required status-check identities; confirm the live ruleset before rollout and obtain maintainer approval for any required-check change. Do not mutate settings or bypass checks. Main/tag publication behavior at a merged SHA requires hosted evidence: a local structural test does not prove GitHub event evaluation. Until #245 is incorporated into #21, source CI must not authorize SDK release automation. Manual module publication still requires selected-module standalone validation. If artifact gating fails on hosted pushes, stop publication and fix or revert the workflow; do not bypass it.
