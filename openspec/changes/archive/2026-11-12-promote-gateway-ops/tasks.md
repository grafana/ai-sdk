# Tasks

## 1. Promotion gate

- [x] 1.1 Extend `release/tooling/gateway-publication-gate.mjs` so a canonical `main` `workflow_dispatch` promotes `GATEWAY_PROMOTED_COMMIT` only after that commit's source CI and its dev publication run succeeded. Verify with `npm test --prefix release/tooling`.

## 2. Ops promotion workflow

- [x] 2.1 Add `.github/workflows/publish-ai-gateway-ops.yml`: a manual dispatch with a `commit` input, the promotion gate, the pinned shared plugin workflow targeting `ops` with the dev settings, and a reviewed ops pin request through the deployment repository's updater. Verify with `mise run test-ci-workflow`.
- [x] 2.2 Test the workflow's trigger, gates, permissions, shared-workflow inputs and pins, and run its pin script against a stub `docker` to check the exact pin request and the version guard. Verify with `mise run test-ci-workflow`.

## 3. Docs

- [x] 3.1 Describe ops promotion, its prerequisites and rollback in `CONTRIBUTING.md`. Verify with `mise run lint-docs`.
