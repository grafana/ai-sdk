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
Remove `last-release-sha` after the first release pull requests have merged and
the manifest and tags provide a clean release boundary for every module.

Root releases are tagged `vX.Y.Z`; nested releases are tagged
`<module-directory>/vX.Y.Z`, which is what the Go tool requires to resolve a
library module inside a repository. Gateway retains `ai-gateway/vX.Y.Z` application
tags without promising standalone `go install` or external Go imports.

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
Library attribution follows actual module ownership: root-owned files such as
`middleware/extract_json.go` release core; nested `middleware/logger/` files release
Logger. A provider-only change can also affect a workspace-built Gateway package.
Automatic Gateway attribution for these linked changes remains an activation blocker
in #245; do not manufacture dependency bumps or assume path-only attribution is enough. Scopes are still worth
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
A library candidate must change exactly one manifest version and its generated
changelog. The validator checks the dedicated bot's immutable user ID
`332654929` (App `5038074`), canonical repository ID `1315458055`, branch, open
non-draft state, and matching title/body/manifest versions. Changelog history must
be preserved and the new entry must equal the release body notes, using the
pinned release-please 17.6.0 format. Release-looking PRs without a version change
fail instead of falling through to ordinary-source handling. Labels grant nothing.
Ordinary source PRs retain #262's candidate-source and provenance gates.

The checkout must be clean and merge the exact live release head with current
canonical main.
Readiness rereads live PR metadata and canonical main after validation to reject
changes during the run. This does not replace strict current-base protection or
merge-queue enforcement: main can still advance after the job finishes. Required
checks, protection changes and post-merge publication authorization remain #245
activation prerequisites. This check does not authorize publication by itself.

For the selected library, readiness resolves its dependency graph from a fresh
public-proxy cache. Replacements are rejected. Each internal prerequisite must
have a tagged version, canonical Git download origin, a matching canonical tag
target, and ancestry in the candidate base. Its owned source snapshot must equal
the base snapshot; compiling against an older dependency is insufficient.
Missing tags, unavailable proxy responses and unverifiable origins fail closed.
Then `scripts/module-policy.sh standalone <root>` downloads, verifies, builds and
tests only the selected module with `GOWORK=off` and readonly manifests.

Snapshot ownership follows the longest registered module root; root middleware
remains root-owned. Snapshots include file paths, Git modes and contents, so
renames and deletions count. Normal exclusions are `.md` and `_test.go` files;
`test`, `tests`, `testdata`, `docs`, `examples`, `openspec`, `release`, `scripts`,
`.github`, `.agents` and `.claude` directory segments; root dotfiles; and the root
workspace, mise, Renovate and release configuration files enumerated in
`scripts/release_readiness.py`. A runtime Go embed directive overrides these
exclusions for all owned files beneath its directory, including hidden assets and
Markdown. This conservative rule may require a release for a non-runtime sibling
change. Nested module ownership still takes precedence. Development-only Go files
do not introduce embed overrides.

## Selected consumer evidence

Readiness also runs `scripts/release_consumer.py`. It builds a private temporary
file proxy containing only the selected candidate module, using the local-only
version `v0.0.0-releasecheck`. It creates no Git tag and uploads nothing. A separate
consumer imports its public packages with no workspace or replacements. The
validator asserts the candidate comes from the module cache and that every
selected dependency version matches the previously verified graph. Prerequisites
continue to come from the public proxy, never neighboring candidate source.

Root additionally runs the existing deterministic simple-text Go scenario against
the archived module. The registered `ai@7.0.127` / provider-utils baseline parses
its SSE with `uiMessageChunkSchema` and assembles the assistant message. Each
provider and middleware runs an explicit credential-free behavioral regression
from its archived package in the consumer graph; the scenario registry fails if
a test disappears or is skipped. The full selected-module suite also runs before
the consumer harness. None of these checks import unrelated downstream source.

To reproduce checks without the mise dependency-install wrapper:

```bash
go test ./internal/releasecheck
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p test_release_readiness.py
bash scripts/module-policy.sh standalone .
PYTHONDONTWRITEBYTECODE=1 python3 scripts/release_consumer.py .
```

Replace `.` with a registered library root for a selected consumer diagnostic.
The consumer command alone does not certify prerequisite tag identity or freshness;
the release readiness entry point performs those checks first. Root's frontend
check installs the locked integration dependencies with pnpm. The candidate source
and registered Vercel baseline are unchanged by this governance slice.

This slice proves the library checks can accept root independently of unready
provider/Bedrock source. It does not prove live App-generated release PR writes,
publication retries/concurrency, required-check protection, dependency-update
progression, or Gateway application readiness. Those remain activation work;
release automation and publication guards stay disabled.

The normal library progression is root release -> Renovate root updates -> directly
dependent library releases -> Renovate provider updates -> Bedrock readiness. The first
root release needs no internal prerequisite tags; nested first releases wait for tagged
prerequisites. Already-merged pseudo-versions remain valid for ordinary source PRs.

Gateway has a separate application contract. Its plugin package uses the selected
workspace revision; library readiness does not substitute for application/package
readiness. Gateway integration, required-check protection, exact-candidate publication
and activation remain follow-up work in #245.

Never move or delete published tags. Correct a bad release with a new version.

## Keep dependent modules on released first-party requirements

The requirement bump cannot live in a release pull request, because the
first-party version it would point at is not published until its own release
pull request merges.

Renovate will own the bump after the release path is activated. Current
[`renovate.json`](../renovate.json) deliberately disables root and internal
module version updates while source PRs use merged pseudo-versions. Before
enabling publication, add and test a narrowly scoped first-party release rule
that overrides those disables, the weekly schedule and the 14-day
`minimumReleaseAge` for tagged prerequisite updates. Do not activate it while
publication remains disabled.

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
3. Add its exact module directory to the root package's
   `exclude-paths` so its commits do not bump the core module.
4. Register a deterministic consumer scenario in `scripts/release_consumer.py`.
5. Run `mise run release-check`.

`mise run release-check` fails when a published module is missing from the
configuration, when a tag shape would not resolve for the Go tool, or when a
published module contains a local `replace` directive.

## First-time repository setup

Three things must be true before the first release.

**A clean history boundary.** The adoption configuration intentionally ignores
pre-adoption release history. After this pull request merges, create
release-worthy changes in dependency order rather than trying to reconstruct
historical changelogs: core first, then the directly dependent modules, then
Bedrock after Renovate has published its prerequisite bumps. Gateway uses its
independent workspace application contract.
Once every module has its first release tag, remove the temporary
`last-release-sha` setting in a normal reviewed pull request.

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
and revocation. Write operations and signed release-PR creation remain untested.

The broker binds issuance to the workflow's file path, so renaming or moving
`.github/workflows/release-please.yml` breaks token minting until that config is
updated.

**Squash merging on `main`.** The `main` ruleset must allow squash merges, and
the repository's squash defaults must be `PR_TITLE` for the commit title and
`PR_BODY` for the message. Merge commits duplicate every changelog entry; a
`COMMIT_OR_PR_TITLE` default makes the release subject depend on how many
commits a branch happened to have.

**Renovate.** Add and validate the first-party tagged-prerequisite rule before
activation. The current internal-module rules intentionally prevent version
updates, and a new rule must explicitly override them and the generic Go delay.
