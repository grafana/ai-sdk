# Tasks

## 1. Pin both dev waves

- [x] 1.1 Change the `update-plugin-version` job in `.github/workflows/publish-ai-gateway-dev.yml` to send one updater request that sets `dev0` and `dev1` to the published version. Verify with `mise run test-ci-workflow`.
- [x] 1.2 Update the exact pin request asserted in `test/conformance/tools/ci-workflow.test.mts` to expect both wave entries. Verify with `mise run test-ci-workflow`.

## 2. Docs

- [x] 2.1 Describe in `CONTRIBUTING.md` the two dev waves, the pin each needs and how to roll both back. Verify with `mise run lint-docs`.
