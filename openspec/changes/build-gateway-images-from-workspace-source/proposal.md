## Why

Required source CI tests coordinated Gateway and SDK/provider/middleware changes at one revision, but Docker builds Gateway against older published module pins and image validation requires standalone Gateway readiness. Thus a green source change can fail publication or ship behavior different from the tested revision. Gateway's supported release artifact is its image, not an independently consumable Go module; issue #263 corrects the container policy introduced in #258/#262.

## What Changes

- Build local, main-SHA and `ai-gateway/vX.Y.Z` images from the exact selected repository revision using the checked-in `go.gateway.work`, root Docker context and one explicit Dockerfile build recipe. Keep the root `go.work` SDK-only, dependency pins and base/tool versions fixed, and exclude secrets and build caches from context and runtime layers.
- Gate publication on source checks, merged-pin ancestry and one-way license/import boundary, multiarch image builds and native smoke at the selected SHA, not standalone Gateway module compilation. Main deployment still requires successful publication; retain existing guards, tag naming and release-please ownership.
- Inventory the target-platform command's actual dependency closure, including local modules attributed to the source SHA with inherited Apache license material and Gateway AGPL/notice, plus resolved external module versions/checksums and notices.
- Update the focused credential-free workflow/container regression and documentation; distinguish image readiness from standalone `GOWORK=off` SDK/provider/middleware module release readiness under #245/#21.

## Capabilities

### New Capabilities
- `gateway-workspace-container`: Source selection, context hygiene, image evidence and target-platform runtime/image validation for Gateway containers.

### Modified Capabilities
- `candidate-source-ci`: Remove standalone Gateway module readiness from image publication while keeping same-revision source and fail-closed image gates.
- `module-validation-modes`: Distinguish workspace-built Gateway images from independent Go-module validation; retain SDK/provider/middleware standalone commands and boundary checks.
- `providerwire-v4-http-contract`: Clarify that Gateway container production uses the explicit workspace without weakening the AGPL/Apache module boundary.
- `upstream-parity-governance`: Clarify artifact-specific standalone publication proof: modules versus Gateway container.

## Impact

Planning affects `ai-gateway/Dockerfile`, root-context Docker ignore, `mise.toml`, `.github/workflows/ci.yml`, license inventory, focused tests in `test/conformance/tools/`, `docs/guides/ai-gateway-container.md`, `AGENTS.md`, `CONTRIBUTING.md` and the four existing specs above. No SDK API/wire or upstream baseline change, release-please implementation, module repin/release bot, module consolidation, licensing relaxation, new mandatory PR standalone gate, duplicate full-CI fixture job, digest promotion or repository protection change is proposed. No images/tags are published by this proposal.
