# Proposal

## Why

The deployment repository now deploys the Gateway plugin to dev in two waves, `dev0` and `dev1`, and pins each wave separately. The single `dev` pin that the dev publish workflow updates no longer exists. The updater leaves a missing key unchanged without reporting an error, so each dev publication now opens no pin pull request and dev stays on its last pinned version (#612).

## What Changes

- After a successful dev publication, the publish workflow pins both dev waves, `dev0` and `dev1`, to the published version in one updater request, so one pull request moves both waves together.
- The gates are unchanged. The workflow pins only the head of `main` and only a version that ends in `+<sha>` of that commit. A newer publication still replaces a pin pull request that has not merged.
- Maintainer documentation describes the two dev waves, the pins they need and how to roll both waves back.

Pinning the dev waves to different versions and changing the ops promotion stay out of scope.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-plugin-publication`: the dev version pin request pins each dev wave to the published version in a single pull request.

## Impact

- **Workflow:** the `update-plugin-version` job in `.github/workflows/publish-ai-gateway-dev.yml` sends one updater entry for each dev wave key of the plugin's versions file.
- **Tests:** `test/conformance/tools/ci-workflow.test.mts` checks the exact pin request with both wave entries.
- **Docs:** `CONTRIBUTING.md`.
- **Prerequisites outside this repository:** the deployment repository must hold a pin for each dev wave. The updater leaves a wave without a pin unchanged.
