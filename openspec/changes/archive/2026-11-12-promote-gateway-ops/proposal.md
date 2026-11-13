# Proposal

## Why

Each `main` push publishes the Gateway plugin to the dev catalog and pins dev to it, but nothing publishes it to the ops catalog. The ops environment always runs an explicitly pinned version, so without an ops catalog version it cannot run the plugin. Ops should run only builds that already passed through dev, and only after a person decides to promote one.

## What Changes

- A manually dispatched ops promotion workflow takes the full SHA of a `main` commit. It runs only from `main` of the canonical repository.
- Before publishing, it requires the commit's successful source CI and plugin validation and a successful dev publication of that commit. A commit whose dev publication was superseded, failed or never ran cannot be promoted.
- It rebuilds the commit through the same pinned shared plugin workflow and settings as the dev publication, targeting the ops catalog with the `grafana_cloud` scope. The version is `<package.json version>+<sha>`, the same version dev published.
- After publishing, it opens a pull request in the deployment repository that pins ops to exactly that version. The pull request does not auto-merge; a person reviews and merges it. It may pin a commit older than the head of `main`, which also covers rollback.
- Maintainer documentation describes promotion, its prerequisites and rollback.

Prod publication, automatic promotion and an approval environment before ops catalog publication stay out of scope.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-plugin-publication`: main-push publication stays dev-only; a gated manual promotion publishes a dev-validated commit to the ops catalog and requests a reviewed ops version pin.

## Impact

- **Workflow:** new `.github/workflows/publish-ai-gateway-ops.yml`. The dev workflow is unchanged.
- **Tooling:** `release/tooling/gateway-publication-gate.mjs` accepts a `workflow_dispatch` promotion of `GATEWAY_PROMOTED_COMMIT` and additionally requires the dev publication run.
- **Tests:** `release/tooling/activation.test.mjs` and `test/conformance/tools/ci-workflow.test.mts`.
- **Docs:** `CONTRIBUTING.md`.
- **Prerequisites outside this repository:** the deployment repository must grant this workflow the updater app's `deploy` permission set for `workflow_dispatch` on `main`, and hold an ops version pin for the plugin. Until the grant exists the pin job fails at token creation; without the pin the updater opens no pull request.
