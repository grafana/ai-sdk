# Proposal

## Why

Each `main` push publishes a new Gateway plugin version to the dev catalog, but nothing runs it. The dev plugin backend for App Platform plugins runs the version pinned for the dev environment in the deployment repository, and that pin changes only through a pull request there. Without automation, every dev rollout is a hand-edited pin (#612).

## What Changes

- After a successful dev publication, the publish workflow opens an auto-merge pull request in the deployment repository that pins the dev environment to exactly the version just published. A newer publication replaces an earlier pull request that has not merged.
- The job refuses to pin a version that does not end in `+<sha>` of the published commit.
- Pull requests, forks and failed or skipped publications open no pull request. Ops and prod pins are unchanged.
- Maintainer documentation describes how a published version reaches dev and how to roll back by pinning an earlier published version.

Provider keys, the production catalog, the access allowlist and ops or prod rollout stay out of scope (#104).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-plugin-publication`: a successful dev publication requests a dev version pin, and the documentation covers rollout and rollback.

## Impact

- **Workflow:** `.github/workflows/publish-ai-gateway-dev.yml` gains an `update-plugin-version` job that uses Grafana's shared `login-to-gar` and `create-github-app-token` actions and the deployment repository's updater image.
- **Tests:** `test/conformance/tools/ci-workflow.test.mts` checks the job's gates, permissions and pins, and runs its script to check the exact pin request.
- **Docs:** `CONTRIBUTING.md`.
- **Prerequisites outside this repository:** the deployment repository must grant this workflow the updater app's `deploy` permission set and hold a dev version pin for the plugin. Until both exist, the job fails at token creation or opens no pull request.
