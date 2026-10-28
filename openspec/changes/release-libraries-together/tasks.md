# Tasks

## 1. Module layout

- [x] 1.1 Add complete local first-party replacements to every tracked `go.mod` and tidy every module
- [x] 1.2 Set every first-party requirement to its manifest version, or the registered initial version, replacing pseudo-versions and placeholders
- [x] 1.3 Remove `go.work`, `go.gateway.work` and their sum files, and ignore `go.work*`
- [x] 1.4 Run mise tasks, CI workflows, the Gateway plugin build and license packaging per module with `GOWORK=off`
- [x] 1.5 Keep the ProviderWire differential harness and integration tests working without workspaces

## 2. Validation

- [x] 2.1 Check replacement completeness and manifest-aligned requirements in `scripts/module-policy.sh`, replacing the merged-pin check, with fixtures in `test/module-policy.sh`
- [x] 2.2 Run the boundary and Gateway-free isolation checks without workspaces
- [x] 2.3 Replace standalone public-proxy commands and workspace verification tasks with per-module validation
- [x] 2.4 Allow only local first-party replacements in `internal/releasecheck`

## 3. Readiness and authorization

- [x] 3.1 Validate combined library release metadata, footprint and changelogs in `scripts/release_readiness.py`
- [x] 3.2 Remove the prerequisite freshness check and source ownership snapshots
- [x] 3.3 Write and accept readiness record schema 2 with all released components in `scripts/release_authorization.py`
- [x] 3.4 Accept `go.mod` requirement-line updates in the footprint and require released libraries' first-party requirements to be tagged or co-released
- [x] 3.5 Remove public-proxy prerequisite resolution, standalone and pre-merge consumer checks from readiness and authorization

## 4. Release tooling

- [x] 4.1 Prepare one combined library release pull request in `release/tooling/activation.mjs`
- [x] 4.2 Set first-party requirements to the candidate manifest versions in the same App-signed preparation commit
- [x] 4.3 Move lifecycle labels once per pull request after all its libraries publish in `release/tooling/publication.mjs`
- [x] 4.4 Block dependents of a blocked co-released prerequisite in authorization and publication
- [x] 4.5 Verify each published version as an external consumer through the public Go proxy
- [x] 4.6 Keep the root bootstrap exercise on its single-root shape

## 5. Configuration and documentation

- [x] 5.1 Release libraries from one pull request in `release-please-config.json`, with Gateway separate, and check it in `internal/releasecheck`
- [x] 5.2 Make `refactor` a releasing changelog section
- [x] 5.3 Disable first-party Renovate updates and confirm third-party updates tidy modules with local replacements
- [x] 5.4 Update `release/README.md`, `release/tooling` docs, `CONTRIBUTING.md`, `ai-gateway/README.md` and architecture docs for local replacements, requirements and the release flow

## 6. Verification

- [x] 6.1 Run the full check suite, integration tests, module-policy fixtures, release tooling and Python tests
- [x] 6.2 Preview the combined release pull request with requirement updates against current `main`
- [x] 6.3 Rehearse a release candidate tree: every module builds and tests offline for first-party modules with untagged co-released requirements
