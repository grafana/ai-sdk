# Tasks

## 1. Pin the published version in dev

- [x] 1.1 Add an `update-plugin-version` job to `.github/workflows/publish-ai-gateway-dev.yml` that runs after `publish`, only in the canonical repository, with `contents: read` and `id-token: write`, and asks the deployment repository's updater to pin `dev` to the published version. Verify with `mise run test-ci-workflow`.
- [x] 1.2 Test the job's gates, permissions and SHA pins, and run its script against a stub `docker` to check the exact pin request and the version guard. Verify with `mise run test-ci-workflow`.

## 2. Docs

- [x] 2.1 Describe in `CONTRIBUTING.md` how a published version reaches dev, the one-time prerequisites and how to roll back. Verify with `mise run lint-docs`.
