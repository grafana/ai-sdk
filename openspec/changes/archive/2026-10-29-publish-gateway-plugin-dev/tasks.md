# Tasks

## 1. Move the backend to `ai-gateway/pkg`

- [x] 1.1 `git mv ai-gateway/cmd/grafana-ai-gateway ai-gateway/pkg` and rewrite import paths from `ai-gateway/cmd/grafana-ai-gateway/internal/` to `ai-gateway/pkg/internal/`. Verify with `git grep 'cmd/grafana-ai-gateway'`, which should only match openspec archives, and with `cd ai-gateway && go build ./... && go vet ./...`.
- [x] 1.2 Set `kinds/config.cue` `goGenPath` to `pkg/internal/` and remove `RootPackagePath` from `Magefile.go`. Verify that `pnpm --filter ./ai-gateway generate:kinds` leaves `git status -- ai-gateway` clean apart from the move, and that `cd ai-gateway && mage -v test` runs the backend tests.
- [x] 1.3 Update `ai-gateway/test/providerwire-v4/gateway-command.test.ts` (`COMMAND_DIR` and the internal-import assertion) and the build roots in `test/module-policy.sh`. Verify that `mise run test-ai-gateway-command` and `mise run test-module-policy` pass.
- [x] 1.4 Update the source links in `docs/concepts/gateway-architecture.md` and `docs/concepts/gateway-glossary.md`, and check their "command" wording against the plugin backend layout. Verify that `mise run lint-docs` passes.

## 2. Single backend build entrypoint

- [x] 2.1 Add a `BuildPlugin` mage target. It builds the Linux amd64 and arm64 backends with `-mod=readonly -trimpath` and then runs `scripts/package-licenses.sh`. It fails when the frontend package is missing. Verify that `cd ai-gateway && mage -v buildPlugin` fails on an empty `dist/`, and succeeds after the frontend build.
- [x] 2.2 Change `package-licenses.sh`: take the backend package from `./pkg`, and set `SOURCE_REVISION` to `GITHUB_SHA` when `GITHUB_ACTIONS=true` and HEAD equals it, or to `local-unverified` otherwise. Remove `AI_GATEWAY_VCS_REF`. Verify a local build writes `local-unverified`. Verify that a run with `GITHUB_ACTIONS=true GITHUB_SHA=$(git rev-parse HEAD)` writes the SHA, and that a run with a mismatching SHA writes `local-unverified`.
- [x] 2.3 Make `mise run build-ai-gateway-plugin` build the frontend and then run `mage -v buildPlugin`. Remove the `AI_GATEWAY_VCS_REF` env from the `plugin-validation` build step, and update its assertion in `test/conformance/tools/ci-workflow.test.mts`. Verify that `mise run build-ai-gateway-plugin`, `mise run validate-ai-gateway-plugin` and `mise run test-ci-workflow` pass.
- [x] 2.4 Update the `CONTRIBUTING.md` Gateway plugin section and the `ai-gateway/README.md` develop-locally notes for the new backend path and build target. Verify the documented commands run as written.

## 3. Dev catalog publication job

- [x] 3.1 Add `.github/workflows/publish-ai-gateway-dev.yml`: a `source-ci` gate job that runs `release/tooling/gateway-publication-gate.mjs`, and a `publish` job that calls `grafana/plugin-ci-workflows/.github/workflows/cd.yml@ci-cd-workflows/v12.0.1` with the inputs, job permissions and workflow-level concurrency from design.md. Leave `ci.yml` without a publication job. Verify with `actionlint` on both files.
- [x] 3.2 Add the gate to the release tooling: `GATEWAY_PUBLICATION_JOBS` (source jobs plus `Validate AI Gateway plugin`), a `jobs` option on `waitForToolingCI`, and the canonical-push checks in the gate script. Verify with `npm test --prefix release/tooling`: the gate rejects missing, failed, cancelled and skipped plugin validation, and non-canonical events.
- [x] 3.3 Extend `test/conformance/tools/ci-workflow.test.mts`:
  - assert the publish workflow's trigger (`main` pushes only), `permissions: {}` and `cancel-in-progress: false` concurrency;
  - assert the gate's repository guard, read-only permissions and script, and the plugin-validation job name it depends on;
  - assert the publish job needs the gate with no `if`, the tag-form `uses`, the exact `with` inputs, the permissions and no `secrets`;
  - assert that ci.yml calls no reusable workflow and no other workflow calls the shared workflows.

  Verify that `mise run test-ci-workflow` passes, and that it fails when you drop `needs`, add `always()` or a `pull_request` trigger, switch `uses` to a branch ref, allow cancel-in-progress, replace the gate command, or rename the plugin-validation job.
- [x] 3.4 Before merge, run the shared workflow's backend commands locally from `ai-gateway/`: `golangci-lint run --timeout=5m` with the pinned version, `mage -v testRace` and `mage -v buildPlugin`. Verify all three pass.
- [x] 3.5 Document dev publication in `CONTRIBUTING.md`:
  - the trigger and version format `<package.json version>+<sha>`;
  - the `grafana_cloud` scope and the gates;
  - serialized runs, and the one-time credential and runner prerequisites;
  - how to find a commit's published version;
  - that publication does not deploy or run the plugin.

  Name no private repositories. In `ai-gateway/README.md`, state that the plugin is in the dev catalog for Grafana Cloud only. Verify that `mise run lint-docs` passes.

## 4. Integration checks

- [x] 4.1 Run `mise run check`, `mise run lint`, `mise run test-ai-gateway`, `mise run test-ai-gateway-frontend` and `mise run test-ai-gateway-e2e`, and verify all pass.
- [x] 4.2 Run `openspec validate publish-gateway-plugin-dev --strict` and verify it passes.

## Workflow follow-up

- Prerequisite (maintainer): create the `grafana-aigateway-app` plugin stub in the dev catalog with the Grafana Labs signature level (done). No runner or credential onboarding is needed.
- After merge, check the first `main` run:
  - the version `<version>+<sha>` is in the dev catalog with the `grafana_cloud` scope;
  - the shared lint, tests, build, validator and trufflehog passed;
  - the published package's `SOURCE_REVISION` equals the commit.

  Record any shared-workflow fix it needs, such as package manager detection.
- Update issue #600 to the rescoped outcome, and open follow-ups for deployment registration, router routing and rollback.
- Archive the change after review.
