## 1. Focused regression contract

- [ ] 1.1 Extend `test/conformance/tools/ci-workflow.test.mts` to assert main/tag image gates, exact SHA, common root-context Dockerfile/workspace recipe, retained source/ancestry/boundary dependencies and fail-closed publication/deployment; remove old standalone-Gateway prerequisite assertions without encoding incidental job scheduling.
- [ ] 1.2 Add one credential-free Docker regression with an uncommitted candidate local SDK/provider behavior change, unchanged older merged pins and readonly manifests; show the locally built Gateway image runs candidate behavior, not downloaded older modules, and standalone Gateway readiness need not pass. Avoid real credentials and invented provider recordings.
- [ ] 1.3 Add context and license-evidence assertions for excluded sentinel secrets/caches, included command embeds, unverified-local source attribution in OCI label and local-module inventory, root Apache/Gateway AGPL notice and resolved external dependencies at target platform. Establish regression failure against the existing published-pin image recipe where feasible.

## 2. Workspace image recipe

- [ ] 2.1 Inventory Gateway command embeds and local workspace dependency files, then add `ai-gateway/Dockerfile.dockerignore` with explicit required source/manifests/checksums/licenses and exclusions; verify it takes effect for the root-context build without imposing Gateway rules on other Dockerfiles or trusting `ai-gateway/.dockerignore`.
- [ ] 2.2 Update `ai-gateway/Dockerfile` to build command source with explicitly selected checked-in `go.gateway.work`, pinned dependencies and readonly manifests for `TARGETOS`/`TARGETARCH`, no floating module updates, and keep the nonroot runtime binary/configuration/required license artifacts only.
- [ ] 2.3 Inventory the command's same-workspace, same-platform `go list -deps` closure including local modules at the verified CI source SHA or unverified-local marker despite empty `.Version`, external resolved versions/checksums, inherited root Apache licensing, Gateway AGPL/NOTICE and existing third-party notices; verify OCI revision matches.
- [ ] 2.4 Change `mise run build-ai-gateway-image` to invoke the explicit Gateway Dockerfile with root context and unverified-local attribution, allowing local working-tree changes without claiming they match HEAD. Keep the same image dependency recipe for CI validation/publication, which pass their verified push SHA.

## 3. Image gates and release boundary

- [ ] 3.1 Update existing `.github/workflows/ci.yml` native/multiarch validation and publisher builds to use the common root-context explicit Dockerfile/workspace recipe at their checked-out `GITHUB_SHA`. Before either job builds, verify a fresh clean checkout with standard Git checks and HEAD equal to `GITHUB_SHA`; use the Gateway Dockerfile-specific narrow context rather than a bespoke Dockerignore-aware file attestation; preserve event guards, amd64/arm64 validation, native readiness smoke, tag naming and exact-revision label.
- [ ] 3.2 Remove `MODULE=ai-gateway mise run verify-published-module` from the container gate while retaining required source/parity/merged-pin and SDK-to-Gateway structural/isolation checks, fail-closed job dependencies and main-only deployment after successful publication; retain SDK/provider/middleware on-demand standalone commands unchanged.

## 4. Documentation and verification

- [ ] 4.1 Update `docs/guides/ai-gateway-container.md`, `AGENTS.md`, `CONTRIBUTING.md` and #245/#21 integration notes to distinguish workspace-built Gateway container/version tag from independently releasable `GOWORK=off` SDK/provider/middleware Go modules and preserve licensing/ancestry policy.
- [ ] 4.2 Run OpenSpec validation, focused workflow/module-policy/regression tests (including local dirty-source attribution and CI revision/clean-check assertions), buildx amd64/arm64 validation, native image readiness smoke and context/layer/inventory inspection; verify failed source/build/smoke paths remain blocking and record any unavailable Docker/buildx proof without claiming it passed.
