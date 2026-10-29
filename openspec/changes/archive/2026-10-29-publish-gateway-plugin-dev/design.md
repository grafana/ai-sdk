# Design

## Context

See proposal.md for motivation. Current state and constraints:

- The backend is the `main` package in `ai-gateway/cmd/grafana-ai-gateway`, with its private packages under `cmd/grafana-ai-gateway/internal/`. `Magefile.go` points the plugin SDK build at that path with `RootPackagePath`.
- `mise run build-ai-gateway-plugin` builds the frontend, runs `mage build:linux build:linuxARM64` with `GOFLAGS=-mod=readonly -trimpath`, then runs `scripts/package-licenses.sh`. The script reads `AI_GATEWAY_VCS_REF` for `SOURCE_REVISION`. Only the repository CI `plugin-validation` job sets that variable.
- The shared workflow (`grafana/plugin-ci-workflows` `cd.yml` → `ci.yml`, tag `ci-cd-workflows/v12.0.1`) runs from `plugin-directory`. Its steps are:
  - detect the package manager, then `install`, `lint`, `typecheck`, `test:ci` and `build`;
  - `golangci-lint run ./pkg/...`;
  - `mage -v testRace`, which the plugin SDK defines as `go test -race ./pkg/...`;
  - `mage -v <backend-build-target>`, defaulting to `buildAll`;
  - zip, sign, trufflehog, an optional plugin validator, then publish.
- Before building, the shared workflow rewrites `package.json` `version` to `<version>+<plugin-version-suffix>`, so the checkout is no longer clean at build time.
- The shared workflow treats only `push`, `workflow_dispatch` and `release` events as trusted for signing and publishing. Its cloud federation is limited to `main` and tags. It fails if it is referenced by commit SHA.
- The `candidate-source-ci` spec requires publication to depend on the source, requirement, boundary and plugin-validation jobs of the same revision.

## Goals / Non-Goals

**Goals:**

- One build entrypoint whose output is identical whether it runs locally, in repository CI or in the shared workflow.
- A publication path that is gated on repository CI. It must not bypass the existing checks, and it must not replace them.

**Non-Goals:**

- Changing Gateway runtime behavior, settings or the plugin package's contents.
- Validating the shared workflow's path before merge. It cannot authenticate outside `main` and tags.
- Changing release-please, Gateway application tags or the library release automation. Dev publication creates no tags or releases. The `multi-module-release-management` "Fail-closed activation" scenario is narrowed to versioned Gateway publication, which stays disabled.

## Decisions

### Move the whole backend directory to `ai-gateway/pkg`

Move `cmd/grafana-ai-gateway` to `pkg` unchanged: `main.go` becomes `pkg/main.go` and the private packages become `pkg/internal/...`. Drop `RootPackagePath` from `Magefile.go`, change `kinds/config.cue` `goGenPath` to `pkg/internal/`, and rewrite import paths.

- **Alternative: merge the private packages into the existing `ai-gateway/internal/`.** Rejected because it widens their visibility from the plugin backend to the whole module, including `catalog`, `providerwire` and the test servers.
- **Alternative: keep the path and override the mage `test`/`testRace` targets.** Rejected because it adds Gateway-specific mage code that every SDK and scaffold update must keep in step. `./pkg` is the documented plugin layout.

### One mage target, `buildPlugin`, owns the backend package step

`BuildPlugin` builds Linux amd64 and arm64 through the SDK build, with `-mod=readonly -trimpath` set by the target itself rather than by callers. It then runs the existing `package-licenses.sh`. The mise task runs the frontend build followed by `mage -v buildPlugin`. The shared workflow gets `backend-build-target: buildPlugin`. `Default` and the `build:*` targets stay as they are for the local development loop.

- **Alternative: port the license script to Go.** Rejected because it adds a lot of code for no behavior change. The script is already covered by the package-content validation.

### Take `SOURCE_REVISION` from the checked-out commit in GitHub Actions

Use `GITHUB_SHA` as the revision when `GITHUB_ACTIONS=true` and `git rev-parse HEAD` equals `GITHUB_SHA`. Otherwise mark the build `local-unverified`. Remove `AI_GATEWAY_VCS_REF`, and keep the existing clean-checkout step in `plugin-validation` as the cleanliness gate for retained artifacts.

- **Alternative: require a clean tree.** Rejected because the shared workflow always dirties `package.json`.
- **Alternative: keep the environment variable.** Rejected because the shared workflow cannot set it.

For pull requests, `GITHUB_SHA` is the merge commit that actions/checkout checks out. That matches today's attribution.

### Publish from a separate push workflow gated on CI evidence

Add `.github/workflows/publish-ai-gateway-dev.yml`, triggered on `push` to `main`, with two jobs:

- **`source-ci` (gate):** runs only in `grafana/ai-sdk` with `contents`, `actions` and `checks` read. It runs `release/tooling/gateway-publication-gate.mjs`, which reuses the release tooling's `waitForToolingCI`/`successfulRun` evidence check. It waits up to 20 minutes for the ci.yml `push` run of the same commit on `main` in this repository, then requires each source job (`ci`, `race`, `docs-lint`, `module-resolution`, `parity-baseline`, `integration-test`, `conformance-test`) and `Validate AI Gateway plugin` to have succeeded.
- **`publish`:** `needs` the gate and calls `cd.yml@ci-cd-workflows/v12.0.1` with:
  - `plugin-directory: ai-gateway`
  - `branch: ${{ github.sha }}`
  - `environment: dev`
  - `scopes: grafana_cloud`
  - `plugin-version-suffix: ${{ github.sha }}`
  - `backend-build-target: buildPlugin`
  - `run-plugin-validator: true`
  - `run-playwright: false`
  - `trigger-argo: false`
  - `golangci-lint-version` equal to the mise pin

  It is granted `contents: write`, `id-token: write`, `attestations: write` and `pull-requests: read` at job level, which `cd.yml` declares and cannot run with less. The workflow-level `permissions: {}` stays.

Alternatives:

- **A job inside `ci.yml`.** Rejected because live release automation uses the whole ci.yml push run as evidence: it waits at most 20 minutes for the run to complete with `success` or `failure`, and rejects `cancelled`. A publish job inside that run lengthens it past the wait and can end it `cancelled` when a newer publication supersedes it.
- **`workflow_run` after CI succeeds.** Rejected because the shared workflow does not trust that event, so it would not sign or publish.
- **An ungated separate workflow.** Rejected because it does not wait for repository CI, which violates `candidate-source-ci`.

The gate reads evidence through the read-only release tooling client, so a failed, skipped, cancelled or missing required job, or a timeout, fails the gate and skips publication. Passing `branch: github.sha` pins the shared checkout to the gated commit, even if `main` moves while the run waits.

### Pin the shared workflow by tag

The repository pins actions by commit SHA, but the shared workflow fails on SHA references, and its cloud federation depends on tag refs. The structural workflow test therefore asserts the exact `@ci-cd-workflows/vX.Y.Z` form, so the exception stays explicit and Renovate-visible.

### Serialize publications

Give the publish workflow a workflow-level concurrency group, `ai-gateway-dev-publish`, with `cancel-in-progress: false`. A started run finishes. Runs enter the group when they are created at push time, and GitHub keeps only the newest pending run, so an older waiting push is superseded and publications follow push order. A deployment that tracks the latest catalog version (#612) relies on that order. A superseded publish run ends `cancelled`, which affects only this workflow, not the ci.yml evidence.

### Skip the shared Playwright run and keep both validators

The plugin-validation gate already runs the end-to-end tests against a pinned Grafana nightly digest, with the feature toggles the plugin needs. The shared run would resolve Grafana from `grafanaDependency` and duplicate that work. The shared plugin validator is enabled because the shared workflow validates what it actually publishes. The repository's pinned validator and its exact-contents check stay in place.

### Documentation placement

`CONTRIBUTING.md` ("What to validate → Gateway plugin") describes:

- the publication path and version format;
- the gates and prerequisites;
- that publication does not deploy the plugin.

`ai-gateway/README.md` gets updated catalog availability. Self-hosted users still build from source. Source links in the architecture doc and glossary follow the move. The docs describe the deployment contract generically and do not name private infrastructure.

## Risks / Trade-offs

- **The shared workflow path cannot run before merge.** It fails closed: a failed publish blocks nothing else. Mitigations:
  - structural workflow tests;
  - a local run of `mage -v testRace` and `mage -v buildPlugin` from `ai-gateway/`;
  - a first-run checklist in tasks.md.
- **Package manager detection in a subdirectory of the root pnpm workspace may fail.** The lockfile is at the repository root; `ai-gateway/package.json` repeats the root `packageManager` because `pnpm/action-setup` reads only the plugin directory's `package.json`. Confirm on the first `main` run. If detection fails, disable the job and follow up with the shared workflow owners.
- **The shared backend steps differ from repository CI.** It lints from `ai-gateway/` with the root `.golangci.yml`, and runs `go test -race` on `./pkg/...` without `-short`. Mitigations:
  - match the `golangci-lint` version to the mise pin;
  - run the same command locally before merge;
  - the race job already covers these packages with `-short`.
- **The first publish needs a catalog stub.** Before the first publish, a `grafana-aigateway-app` plugin stub with the Grafana Labs signature level must exist in the dev catalog (or in prod, from which the shared workflow copies it to dev). Without it, the shared workflow's stub check fails the publish run; merges, the required jobs and the ci.yml run are unaffected. No per-repository onboarding is needed otherwise: the shared workflow's GCP access is bound to the shared workflow's own tag-pinned identity, its catalog tokens are common CI secrets, and its self-hosted runners (`ubuntu-arm64-small`, `ubuntu-x64-small`, `ubuntu-x64-large`) serve every repository in the organization, public ones included.
- **The repository Actions policy applies to the whole shared call tree.** GitHub validates every job of `cd.yml` and its nested workflows when the calling workflow starts, so one denied action version anywhere in the shared workflow fails the whole run at startup. That was how ci.yml failed on pull requests while publication lived there. `ci-cd-workflows/v9.0.0` hit this through a denied `create-github-app-token` version. Check any pin upgrade against the policy before merging it.
- **Version order depends on publish order.** Versions that differ only in `+<sha>` have equal semver precedence, so "latest" depends on publish order. Workflow-level serialization keeps publish order equal to push order; see "Serialize publications".
- **The gate fails closed slowly, and does not wait for re-runs.** When source CI fails, the gate polls for its full 20-minute budget before failing. Because of the workflow-level group, later pushes wait behind it, and pushes that arrive in that window collapse to the newest one. If CI passes only after a re-run past that window, the commit is not published automatically; a maintainer re-runs its publish run, and only while the commit is still the newest on `main`, so push order holds.
- **The catalog grows by one version per `main` push.** This is acceptable for dev.

## Migration Plan

1. Create the `grafana-aigateway-app` stub in the dev catalog with the Grafana Labs signature level (done).
2. Merge. The first `main` push publishes `<version>+<sha>`. Check the run summary, the catalog entry and the `SOURCE_REVISION` of the published package.
3. To roll back, remove or disable the job. Versions already published stay in the dev catalog until an admin removes them.
