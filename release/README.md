# Release runbook

The repository publishes the core SDK, AI Gateway, each provider, and each
middleware integration as independently versioned Go modules. Releases are
driven by [release-please](https://github.com/googleapis/release-please): merged
Conventional Commits become versions, changelogs, tags, and GitHub Releases
without a maintainer calculating anything.

Configuration lives in two files:

- [`release-please-config.json`](../release-please-config.json) registers every
  published module, its component name, and its tag shape.
- [`.release-please-manifest.json`](../.release-please-manifest.json) records
  the last released version of each module. release-please owns this file.

The initial adoption deliberately starts after commit `1ac1e91`. Before that
boundary the repository used merge commits, so scanning the old history would
attribute both branch commits and their merge commits to the first generated
changelogs. `bootstrap-sha` applies that boundary to modules without a release;
the temporary `last-release-sha` applies it to the already-tagged root module.
Remove `last-release-sha` after the first release pull requests have merged and
the manifest and tags provide a clean release boundary for every module.

Root releases are tagged `vX.Y.Z`; nested releases are tagged
`<module-directory>/vX.Y.Z`, which is what the Go tool requires to resolve a
module inside a repository.

## Record release intent in normal pull requests

Release intent is the pull request title. Pull requests are squash-merged with
the title as the commit subject, so that title is the only thing release-please
reads. Nothing else is needed in a feature pull request:

| Pull request title | Effect on the touched modules |
| ------------------ | ----------------------------- |
| `fix: ...`, `perf: ...` | patch release |
| `feat: ...` | minor release |
| `feat!: ...` or a `BREAKING CHANGE:` footer | major release |
| `chore: ...`, `ci: ...`, `docs: ...`, `refactor: ...`, `test: ...` | no release |

A change is attributed to a module by the files it touches, not by its scope.
A pull request under `providers/openai/` releases only that provider; one that
also touches the repository root releases core as well. Scopes are still worth
writing because they appear in the changelog.

Squash merging is load-bearing. Under merge commits both the branch commit and
the merge commit reach `main` with the same subject, and release-please only
collapses the pair when the pull request contains exactly one commit, so every
larger pull request produces duplicate changelog entries.

While the modules are in the `alpha` channel, every release-worthy commit
advances the prerelease counter, for example `v0.1.0-alpha.1` to
`v0.1.0-alpha.2`. The base version only moves when the channel is graduated.

Validate the configuration itself after adding a module or changing tag
settings:

```bash
mise run release-check
```

## Preview the next release

Preview what release-please would propose from the current `main`, without
changing anything:

```bash
mise run release-preview
```

This runs the release-please CLI in dry-run mode against the GitHub API using
your `gh` credentials. It prints the proposed release pull request, including
each module's next version and changelog entry.

The CLI reads the configuration and commit history from GitHub, not from your
working tree, so the preview reflects the configuration on `main`. To preview a
configuration change before it merges, push the branch and add
`--target-branch=<branch>` to the command.

## Publish a release

Every module gets its own release pull request, on a branch named
`release-please--branches--main--components--<component>`. Every push to `main`
refreshes the pull requests for the modules that change. Each one holds the
calculated version, the generated changelog, and the manifest update for a
single module, and the full CI suite runs against it.

Merging a release pull request makes release-please create that module's tag
and GitHub Release. Modules with nothing pending have no pull request, so the
set of open release pull requests is the set of releasable modules.

**Release prerequisites before their dependents.** Every nested module requires
a published core version. Bedrock also requires the OpenAI provider, while the
AI Gateway requires the Anthropic and OpenAI-compatible providers plus the
Agent Observability, Logger, and Prometheus middleware modules. The order is:

1. Merge the root release pull request. The core tag publishes.
2. Renovate opens a `fix(deps)` pull request updating the root requirement in
   every nested module. It auto-merges.
3. Merge release pull requests for every module whose first-party requirements
   are now published. On the initial bootstrap this wave includes Anthropic,
   Grafana, OpenAI, OpenAI-compatible, Agent Observability, Enrichment, Logger,
   and Prometheus; Bedrock and AI Gateway still wait for their additional
   prerequisites.
4. Renovate updates those first-party requirements in Bedrock and AI Gateway.
5. Merge the Bedrock and AI Gateway release pull requests after their complete
   published dependency sets resolve.

The `module-resolution` check enforces this on every pull request rather than
leaving it to discipline. It tests every published module with `GOWORK=off`,
`-mod=readonly`, and the public Go proxy — that is, against the versions its
`go.mod` actually pins rather than the working tree that `go.work` would
otherwise supply. A release pull request that needs an unpublished first-party
API stays red until the corresponding Renovate update lands.

Never move or delete a published tag. Correct a bad release with a new version.

## Keep dependent modules on released first-party requirements

The requirement bump cannot live in a release pull request, because the
first-party version it would point at is not published until its own release
pull request merges.

Renovate owns the bump instead. Its rule is the last entry in
[`renovate.json`](../renovate.json) because it has to override the weekly
schedule and the 14-day `minimumReleaseAge` that the generic Go rules apply.

Two consequences are worth knowing. Renovate commits through the GitHub API, so
its commits are signed and satisfy the organization's signed-commit ruleset,
which a workflow pushing with `git` cannot do. And because Renovate labels the
bump `fix(deps)`, merging it makes each dependent module's next release include
the new first-party requirements.

## Force a release for an unchanged module

release-please only proposes a release for a module with release-worthy
commits. A module that has never had one — `middleware/enrichment` at the time
of writing — gets no initial tag and resolves to a pseudo-version for
consumers. To release it, land a pull request that touches a file inside it,
for example a changelog note or a documentation comment, with a release-worthy
title:

```text
fix(middleware/enrichment): refresh released module metadata
```

To pin an exact version instead of the calculated one, add a footer to the pull
request body:

```text
Release-As: 0.2.0
```

## Graduate the alpha channel

The channel lives in `release-please-config.json`. The versioning strategy is
repository-wide:

```json
"versioning": "prerelease",
"prerelease": true
```

The channel name is per package, because the configuration schema only accepts
`prerelease-type` inside a package entry:

```json
"providers/openai": { "prerelease-type": "alpha" }
```

To move to `beta`, change `prerelease-type` in every package that moves. To
graduate to stable versions, set `"prerelease": false`; the next release drops
the prerelease suffix, so `v0.1.0-alpha.7` becomes `v0.1.0`. Both are reviewed
configuration changes rather than release-time flags, so the channel is always
visible in `main`.

`mise run release-check` fails if a registered package is missing its
`prerelease-type`.

## Add a public module

1. Add the module's `go.mod`.
2. Register it in `release-please-config.json` with a `component` equal to its
   repository directory and an `initial-version`.
3. Add its directory (or its parent directory) to the root package's
   `exclude-paths` so its commits do not bump the core module.
4. Run `mise run release-check`.

`mise run release-check` fails when a published module is missing from the
configuration, when a tag shape would not resolve for the Go tool, or when a
published module contains a local `replace` directive.

## First-time repository setup

Three things must be true before the first release.

**A clean history boundary.** The adoption configuration intentionally ignores
pre-adoption release history. After this pull request merges, create
release-worthy changes in dependency order rather than trying to reconstruct
historical changelogs: core first, then the directly dependent modules, then
Bedrock and AI Gateway after Renovate has published their prerequisite bumps.
Once every module has its first release tag, remove the temporary
`last-release-sha` setting in a normal reviewed pull request.

**A release app token.** release-please needs a token that can open pull
requests which then trigger the required CI checks. The default `GITHUB_TOKEN`
cannot do this: pull requests it creates do not start workflow runs, so a
release pull request could never satisfy branch protection.

The workflow mints the token through
[`grafana/shared-workflows/actions/create-github-app-token`](https://github.com/grafana/shared-workflows/tree/main/actions/create-github-app-token),
which exchanges the job's OIDC identity for a short-lived installation token via
Vault's [GitHub App Token Broker](https://enghub.grafana-ops.net/docs/default/component/deployment-tools/platform/vault/github-app-token-broker/),
so no app private key is stored in this repository.

The proposed app is `grafana-plugins-platform-bot`, the same one
`grafana/agento11y` and `grafana/plugin-tools` use for their release workflows.
Platform Productivity must confirm that it is appropriate for this repository.
If approved, two things must be provisioned in `grafana/deployment_tools`
before the first run:

1. `grafana/ai-sdk` added to
   `terraform/repositories/plugin-ci-workflows/plugins-platform-bot-users.txt`,
   plus the matching app installation granted on the repository. The
   installation itself is a manual step; the file only tracks it.
2. `terraform/repositories/ai-sdk/github-app-configs/config.yaml` declaring this
   workflow, bound to `branch: main` and `event_name: push`, with
   `contents: write`, `pull_requests: write`, and `issues: write` (release-please
   manages lifecycle labels through the issues API).

The broker binds issuance to the workflow's file path, so renaming or moving
`.github/workflows/release-please.yml` breaks token minting until that config is
updated.

**Squash merging on `main`.** The `main` ruleset must allow squash merges, and
the repository's squash defaults must be `PR_TITLE` for the commit title and
`PR_BODY` for the message. Merge commits duplicate every changelog entry; a
`COMMIT_OR_PR_TITLE` default makes the release subject depend on how many
commits a branch happened to have.

**Renovate.** The `first-party module requirements` rule in
[`renovate.json`](../renovate.json) must stay the last `packageRules` entry, or
the generic Go rules will hold dependent modules on stale prerequisites for up
to two weeks.
