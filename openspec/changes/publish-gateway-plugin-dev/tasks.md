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

- [x] 3.1 Add the `publish-ai-gateway-dev` job to `.github/workflows/ci.yml`. It calls `grafana/plugin-ci-workflows/.github/workflows/cd.yml@ci-cd-workflows/v9.0.0` with the inputs, `needs`, main-only guard, job permissions and concurrency from design.md. Verify with `actionlint` (or the repository's workflow lint) on the file.
- [x] 3.2 Extend `test/conformance/tools/ci-workflow.test.mts`:
  - add the job to the expected job set;
  - assert its `needs` cover the source jobs and `plugin-validation`;
  - assert the main-only canonical guard (no tags, no PRs);
  - assert the tag-form `uses`, the exact `with` inputs and the permissions;
  - assert `cancel-in-progress: false` concurrency;
  - assert that no other job or workflow calls `cd.yml`;
  - replace the "no publication jobs" assertion with these checks.

  Verify that `mise run test-ci-workflow` passes, and that it fails when you drop a `needs` entry or switch `uses` to a branch ref.
- [x] 3.3 Before merge, run the shared workflow's backend commands locally from `ai-gateway/`: `golangci-lint run --timeout=5m` with the pinned version, `mage -v testRace` and `mage -v buildPlugin`. Verify all three pass.
- [x] 3.4 Document dev publication in `CONTRIBUTING.md`:
  - the trigger and version format `<package.json version>+<sha>`;
  - the `grafana_cloud` scope and the gates;
  - serialized runs, and the one-time catalog stub and credential prerequisites;
  - how to find a commit's published version;
  - that publication does not deploy or run the plugin.

  Name no private repositories. In `ai-gateway/README.md`, state that the plugin is in the dev catalog for Grafana Cloud only. Verify that `mise run lint-docs` passes.

## 4. Integration checks

- [ ] 4.1 Run `mise run check`, `mise run lint`, `mise run test-ai-gateway`, `mise run test-ai-gateway-frontend` and `mise run test-ai-gateway-e2e`, and verify all pass.
- [x] 4.2 Run `openspec validate publish-gateway-plugin-dev --strict` and verify it passes.

## Workflow follow-up

- Prerequisite (maintainer, with elevated catalog access): create the `grafana-aigateway-app` plugin stub in the dev catalog, and confirm that the repository may use the shared signing and publishing credentials.
- After merge, check the first `main` run:
  - the version `<version>+<sha>` is in the dev catalog with the `grafana_cloud` scope;
  - the shared lint, tests, build, validator and trufflehog passed;
  - the published package's `SOURCE_REVISION` equals the commit.

  Record any shared-workflow fix it needs, such as package manager detection.
- Update issue #600 to the rescoped outcome, and open follow-ups for deployment registration, router routing and rollback.
- Archive the change after review.
