# Proposal

## Why

The Gateway app plugin (`grafana-aigateway-app`) is built and validated on every push, but no catalog contains it, so no Grafana Cloud environment can install or run it. Grafana's shared plugin CI/CD workflow publishes App Platform plugins to the dev catalog. It cannot build the Gateway today because it tests and builds the backend from `./pkg`, and the Gateway backend lives in `ai-gateway/cmd/grafana-ai-gateway`.

Issue #600 is rescoped to this outcome: every `main` push publishes a traceable, validated plugin version to the dev catalog. Registering a plugin backend deployment, routing the API group and running the plugin in dev follow separately.

## What Changes

- Move the plugin backend from `ai-gateway/cmd/grafana-ai-gateway` to `ai-gateway/pkg`, the standard plugin backend root. Runtime behavior does not change.
- Add one mage target that builds the Linux amd64 and arm64 backends and adds the license inventory and `SOURCE_REVISION` to the package. The local build, repository CI and the shared workflow all use it.
- Derive `SOURCE_REVISION` from the checked-out commit in GitHub Actions, instead of an environment variable that only repository CI sets. Local builds stay marked as unverified development builds.
- Add a dev publication workflow: on every `main` push of the canonical repository, wait until that push's required CI run shows the source, boundary and plugin-validation jobs succeeded, then call the shared `cd.yml` workflow, pinned to a release tag. The required CI run itself is unchanged, so release automation keeps the same evidence. It publishes version `<package.json version>+<commit SHA>` to the dev catalog with the `grafana_cloud` scope and the shared plugin validator enabled.
- Document how a `main` commit becomes a dev catalog version, the prerequisites, and what publication does not do.

## Capabilities

### New Capabilities

- `gateway-plugin-publication`: when and how a validated Gateway plugin package is published to a plugin catalog: the trigger, the version identity, the catalog scope, the gates it depends on and its boundaries with deployment and versioned releases.

### Modified Capabilities

- `gateway-plugin-packaging`: the backend is built from the standard plugin backend root through one build entrypoint. Source attribution comes from the checked-out commit in CI. The "SHALL NOT publish" boundary gives way to the publication capability.
- `multi-module-release-management`: the fail-closed activation scenario covers versioned Gateway publication, which stays disabled; dev catalog publication of `main` builds is allowed.
- `candidate-source-ci`: a Gateway publication workflow now exists. It publishes only after the same push's required CI run shows the source, boundary and plugin-validation jobs succeeded, and it never runs inside or changes that run.

## Impact

- **Code:** `ai-gateway/cmd/grafana-ai-gateway/**` moves to `ai-gateway/pkg/**`, and Go import paths change from `.../ai-gateway/cmd/grafana-ai-gateway/internal/...` to `.../ai-gateway/pkg/internal/...`. Also affected: `ai-gateway/Magefile.go`, `ai-gateway/kinds/config.cue` (`goGenPath`), `ai-gateway/scripts/package-licenses.sh`, `ai-gateway/test/providerwire-v4/gateway-command.test.ts` and `test/module-policy.sh`.
- **Build and CI:** the `build-ai-gateway-plugin` mise task, `.github/workflows/ci.yml` (the build no longer sets `AI_GATEWAY_VCS_REF`), a new `.github/workflows/publish-ai-gateway-dev.yml`, a publication gate in `release/tooling/` that reuses the release evidence check, and the structural workflow test `test/conformance/tools/ci-workflow.test.mts`.
- **External systems:** `grafana/plugin-ci-workflows` (`cd.yml`, pinned by tag), plugin signing and the dev plugin catalog. Before the first publish, a `grafana-aigateway-app` plugin stub with the Grafana Labs signature level must exist in the dev catalog (or in prod, from which the shared workflow copies it to dev).
- **Docs:** `CONTRIBUTING.md` (Gateway plugin validation and publication), `ai-gateway/README.md` (catalog availability), and source links in `docs/concepts/gateway-architecture.md` and `docs/concepts/gateway-glossary.md`.
- **Out of scope:** registering the plugin backend deployment and API group, router routing for a test stack, running the plugin in dev, rollback by pinning versions, provider keys and the model catalog (#104), and versioned releases to ops or prod.
