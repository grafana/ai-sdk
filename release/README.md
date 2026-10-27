# Release runbook

The repository publishes the core SDK, providers, and middleware as independently
versioned Go libraries. Gateway is an independently versioned plugin application;
its Go module is an internal build and license boundary. Releases are driven by [release-please](https://github.com/googleapis/release-please): merged
Conventional Commits become versions, changelogs, tags, and GitHub Releases
without a maintainer calculating anything.

Configuration lives in two files:

- [`release-please-config.json`](../release-please-config.json) registers every
  release component, its name, and its tag shape.
- [`.release-please-manifest.json`](../.release-please-manifest.json) records
  the last released version of each module. release-please owns this file.

The initial adoption deliberately starts after commit `1ac1e91`. Before that
boundary the repository used merge commits, so scanning the old history would
attribute both branch commits and their merge commits to the first generated
changelogs. `bootstrap-sha` applies that boundary to modules without a release;
the temporary `last-release-sha` applies it to the already-tagged root module.
The activated preparation driver ignores `last-release-sha` in memory and uses
actual releases as its boundary, retaining `bootstrap-sha` for never-released modules.
The committed config is retained for bootstrap candidate evidence.

Root releases are tagged `vX.Y.Z`; nested releases are tagged
`<module-directory>/vX.Y.Z`, which is what the Go tool requires to resolve a
library module inside a repository. Gateway retains `ai-gateway/vX.Y.Z` application
tags without promising standalone `go install` or external Go imports.

## Release libraries together

release-please proposes every pending library in one release pull request,
`chore: release main` on `release-please--branches--main`. It carries each
library's version and changelog entry, the manifest update, and the first-party
requirement updates described below. Merging it tags every released library at the
merge commit and creates one GitHub Release per library. Gateway keeps its own
release pull request. Only libraries with release-worthy commits are released.

Every `go.mod` replaces each first-party module in its build graph with that
module's directory, and requires it at its version in the manifest. Go applies
`replace` directives only in the main module, so consumers resolve the published
tags, while every build in this repository uses the source at the same revision.
Release preparation sets those requirements to the versions being released, so a
provider that uses unreleased root behavior is released together with root and
requires the new root version, although that tag does not exist until the merge.
Publication tags prerequisites before their dependents.

## First-party requirements

Every first-party requirement equals the required module's version in
[`.release-please-manifest.json`](../.release-please-manifest.json), or its
registered `initial-version` if it has never been released. Pseudo-versions are not
used. `mise run verify-module-requirements` enforces this together with complete
local replacements, including transitive ones, and runs in CI. After adding a
first-party dependency or a module, run:

```bash
mise run tidy
```

It writes the replacements and requirement versions and tidies every module. There
are no Go workspaces; every task runs inside its module with `GOWORK=off`. Renovate
does not update first-party modules.

## Record release intent in normal pull requests

Release intent is the pull request title. Pull requests are squash-merged with
the title as the commit subject, so that title is the only thing release-please
reads. Nothing else is needed in a feature pull request:

| Pull request title | Effect on the touched modules |
| ------------------ | ----------------------------- |
| `fix: ...`, `perf: ...`, `refactor: ...` | patch release |
| `feat: ...` | minor release |
| `feat!: ...` or a `BREAKING CHANGE:` footer | major release |
| `chore: ...`, `ci: ...`, `docs: ...`, `test: ...` | no release |

A change is attributed to a module by the files it touches, not by its scope.
Library attribution follows actual module ownership: root-owned files such as
`middleware/extract_json.go` release core; nested `middleware/logger/` files release
Logger. A provider-only change can also affect the Gateway plugin package, which is
built from same-revision source.
Gateway attribution and delivery are separate from library activation; do not
manufacture dependency bumps or assume path-only attribution is enough. Scopes are still worth
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

This runs release-please 17.6.0, matching the pinned action dependency, in dry-run mode against the GitHub API using
your `gh` credentials. It prints the proposed release pull request, including
each module's next version and changelog entry.

The CLI reads the configuration and commit history from GitHub, not from your
working tree, so the preview reflects the configuration on `main`. To preview a
configuration change before it merges, push the branch and add
`--target-branch=<branch>` to the command.

For a locked preview of one component, including generated file contents and
the inspected source revision, use the [release proposal preview tool](tooling/README.md).
It rejects write requests and does not authorize publication.

## Publication status and readiness

Release automation is inactive pending #245 activation. The release-please job
is unconditionally skipped and cannot prepare release PRs, create tags, or
publish GitHub Releases. A separate token smoke test runs on main pushes changing
`.github/workflows/release-please.yml`. It mints a brokered token, checks that its
repository scope is exactly `grafana/ai-sdk`, and reads the main branch reference.
It performs no repository writes and does not check out repository code. The
pinned token action attempts token revocation during post-job cleanup; inspect
that step for confirmation or warnings. Successful read access does not prove
write permissions or signed release-PR creation.

The broker only authorizes main push events. Manual dispatch skips both jobs;
rerun the eligible push run to repeat the smoke test. The pinned
action also retains `skip-github-release: true` as a second guard. #245 must
complete Gateway attribution and artifact readiness before enabling the
release-please job. Review pending release PRs and already-merged candidates before
removing the publication guard; activation must not publish an unvalidated version.

The `release-readiness` check is informational until maintainers make it required.
A library candidate may change only the manifest versions of the libraries it
releases, their generated changelogs, and first-party requirement lines set to the
candidate manifest versions. The validator checks the dedicated bot's
immutable user ID `332654929` (App `5038074`), canonical repository ID
`1315458055`, branch, open non-draft state, and matching title/body/manifest
versions. A combined release body must contain exactly one release notes section
per released library. Changelog history must be preserved and each new entry must
equal that library's release notes, using the pinned release-please 17.6.0 format. Release-looking PRs without a version change
fail instead of falling through to ordinary-source handling. Labels grant nothing.
Ordinary source PRs retain #262's candidate-source and provenance gates.

The checkout must be clean and merge the exact live release head with current
canonical main.
Readiness rereads live PR metadata and canonical main after validation to reject
changes during the run. This does not replace strict current-base protection or
merge-queue enforcement: main can still advance after the job finishes. Required
checks, protection changes and post-merge publication authorization remain #245
activation prerequisites. This check does not authorize publication by itself.

The release pull request's CI builds and tests every module against the source
being released, which is the behavioral evidence. For each released library,
readiness additionally requires every first-party requirement to be a canonical
tag already on `main` or a version released by the same pull request.

## Published version verification

After tagging a library, publication runs `scripts/release_consumer.py`. It creates
a fresh consumer module that requires the new version through the public Go proxy
and checksum database, with no workspace or replacements, checks that the published
version is selected from the module cache, builds every public package, and runs a
credential-free behavioral scenario. Root runs the deterministic simple-text Go
scenario, and the registered `ai@7.0.127` baseline parses its SSE with
`uiMessageChunkSchema`. The proxy is queried only after the tag exists, so a
missing version is never cached. A failure is reported in the publication result;
fix it forward with a new release.

To reproduce the checks locally:

```bash
go test ./internal/releasecheck
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p 'test_release_*.py'
mise run verify-module-requirements
PYTHONDONTWRITEBYTECODE=1 python3 scripts/release_consumer.py providers/openai v0.1.0-alpha.1
```

Only run the consumer check for a version that is already tagged.

Library verification, App-signed PR creation and read-only merged-root authorization
have live evidence in #468. Publication/recovery tooling is tested with mocked writes.
The [activation rollout](tooling/ACTIVATION.md) records the remaining settings, root
publication, external consumption and dependency progression evidence. Gateway image
work is excluded. Release writes default off until explicit activation.

Gateway has a separate application contract: main and versioned plugin packages build
from same-revision source through the replacements in `ai-gateway/go.mod` and pass plugin build, licensing and validator checks,
with no dependency on prior SDK tags. CI keeps the
validated package of eligible pushes as a workflow artifact; this repository does not
publish the plugin yet. Its release also updates `ai-gateway/package.json`, the plugin's
version source. Gateway release readiness fails closed pending a plugin publication
workflow. Gateway integration is separate: library readiness does not substitute for
application/package readiness, and Gateway attribution and delivery do not gate library
activation. Library protection settings and live publication follow the activation
rollout under #245.

Never move or delete published tags. Correct a bad release with a new version.

## Force a release for an unchanged module

release-please only proposes a release for a module with release-worthy
commits. A module that has never had one gets no initial tag. To release it, land a pull request that touches a file inside it,
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
3. Add its exact module directory to the root package's
   `exclude-paths` so its commits do not bump the core module.
4. Register a deterministic consumer scenario in `scripts/release_consumer.py`.
5. Run `mise run tidy`, then `mise run release-check`.

`mise run release-check` fails when a published module is missing from the
configuration or when a tag shape would not resolve for the Go tool.

## First-time repository setup

Three things must be true before the first release.

**A clean history boundary.** The adoption configuration intentionally ignores
pre-adoption release history. After this pull request merges, create
release-worthy changes rather than trying to reconstruct historical changelogs.
Gateway uses its independent application contract.
Preparation ignores the temporary `last-release-sha` setting in memory so published
modules advance independently of modules that have no first tag.

**A release app token.** release-please needs a token that can open pull
requests which then trigger the required CI checks. The default `GITHUB_TOKEN`
cannot do this: pull requests it creates do not start workflow runs, so a
release pull request could never satisfy branch protection.

The workflow mints the token through
[`grafana/shared-workflows/actions/create-github-app-token`](https://github.com/grafana/shared-workflows/tree/main/actions/create-github-app-token),
which exchanges the job's OIDC identity for a short-lived installation token via
a token broker, so no App private key is stored in this repository.

The dedicated App is `grafana-ai-sdk-release`, owned by the AI SDK maintainers
and installed only on `grafana/ai-sdk`. Its broker authorization is restricted to
this workflow on main push events, with contents, pull-request and issue write
permissions. PR #465's smoke run verified token issuance, repository read scope,
and revocation. The controlled exercise also proved signed App PR creation and triggered readiness
checks through merged root PR #490. Live publication proof remains pending.

The broker binds issuance to the workflow's file path, so renaming or moving
`.github/workflows/release-please.yml` breaks token minting until that config is
updated.

**Squash merging on `main`.** The `main` ruleset must allow squash merges, and
the repository's squash defaults must be `PR_TITLE` for the commit title and
`PR_BODY` for the message. Merge commits duplicate every changelog entry; a
`COMMIT_OR_PR_TITLE` default makes the release subject depend on how many
commits a branch happened to have.

**Renovate.** The final rule in [`renovate.json`](../renovate.json) disables
first-party module updates; release pull requests update those requirements.
