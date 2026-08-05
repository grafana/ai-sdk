# Release runbook

The repository publishes the core SDK, each provider, and each middleware
integration as independently versioned Go modules. Releases are driven by
[release-please](https://github.com/googleapis/release-please): merged
Conventional Commits become versions, changelogs, tags, and GitHub Releases
without a maintainer calculating anything.

Configuration lives in two files:

- [`release-please-config.json`](../release-please-config.json) registers every
  published module, its component name, and its tag shape.
- [`.release-please-manifest.json`](../.release-please-manifest.json) records
  the last released version of each module. release-please owns this file.

Root releases are tagged `vX.Y.Z`; nested releases are tagged
`<module-directory>/vX.Y.Z`, which is what the Go tool requires to resolve a
module inside a repository.

## Record release intent in normal pull requests

Release intent is the commit message. Nothing else is needed in a feature
pull request:

| Commit subject | Effect on the touched modules |
| -------------- | ----------------------------- |
| `fix: ...`, `perf: ...` | patch release |
| `feat: ...` | minor release |
| `feat!: ...` or a `BREAKING CHANGE:` footer | major release |
| `chore: ...`, `ci: ...`, `docs: ...`, `refactor: ...`, `test: ...` | no release |

A commit is attributed to a module by the files it touches, not by its scope.
A commit under `providers/openai/` releases only that provider; a commit that
also touches the repository root releases core as well. Scopes are still worth
writing because they appear in the changelog.

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

Publication is automatic and has two steps:

1. Every push to `main` refreshes the `chore(main): release Go modules` pull
   request. It contains the calculated versions, the generated changelogs, and
   the manifest update. Review it like any other pull request; the full CI suite
   runs against it.
2. Merging that pull request makes release-please create every tag and GitHub
   Release for the modules it contains.

To release only some modules, merge the release pull request when only those
modules have pending commits, or temporarily remove entries from the release
pull request branch. There is no selective release command; batching is
controlled by what has landed on `main`.

Never move or delete a published tag. Correct a bad release with a new version.

## Keep nested modules on the released core

Nested modules require a published core version. Because the core tag does not
exist until the release pull request merges, the requirement bump cannot be part
of that pull request without making it unresolvable.

Renovate owns that bump. Once the core tag is published, it opens a
`fix(deps): update core module requirement` pull request that updates `go.mod`
and `go.sum` for every nested module. Its rule is the last entry in
[`renovate.json`](../renovate.json) because it has to override the weekly
schedule and the 14-day `minimumReleaseAge` that the generic Go rules apply.

Two consequences are worth knowing. Renovate commits through the GitHub API, so
its commits are signed and satisfy the repository's signed-commit rule, which a
workflow pushing with `git` cannot do. And because Renovate labels the bump
`fix(deps)`, merging it makes each nested module's next release include the new
core requirement.

## Force a release for an unchanged module

release-please only proposes a release for a module with release-worthy
commits. To release a module that has not changed, land a commit that touches a
file inside it, for example a changelog note or a documentation comment, with a
release-worthy type:

```text
fix(providers/bedrock): refresh released module metadata
```

To pin an exact version instead of the calculated one, add a footer to that
commit:

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

release-please needs a token that can open pull requests which then trigger the
required CI checks. The default `GITHUB_TOKEN` cannot do this: pull requests it
creates do not start workflow runs, so a release pull request could never
satisfy branch protection.

Create a GitHub App for the repository with `contents: write` and
`pull_requests: write`, then configure:

- repository variable `RELEASE_APP_ID`
- repository secret `RELEASE_APP_PRIVATE_KEY`
