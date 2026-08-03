## Context

The repository publishes one root Go module and nine provider or middleware
modules from subdirectories. Go requires the root tag to be `vX.Y.Z` and each
nested tag to be `<directory>/vX.Y.Z`. Nested modules require the root module,
local development resolves through `go.work`, and the `module-resolution` CI job
verifies every published module with `GOWORK=off` against the public module
proxy.

Contributors already write Conventional Commits, pull requests are merged with
merge commits, and the repository already relies on hosted automation
(Renovate, mise-based CI). The open question is whether release intent should be
a second hand-written artifact or a derivative of the commit history.

## Goals / Non-Goals

**Goals:**

- Publish independent versions per module without a maintainer calculating one.
- Keep the release reviewable: a pull request that shows every version and
  changelog entry before anything is tagged.
- Keep every state on `main` verifiable by the existing CI jobs, including
  module resolution against the proxy.
- Support the current `alpha` prerelease channel and an explicit graduation.
- Give agents a low-freedom workflow that cannot publish.

**Non-Goals:**

- Owning version arithmetic, changelog rendering, tag creation, or GitHub
  Release creation in repository code.
- Selective, ad-hoc release batching from the command line.
- Publishing binaries or container images.
- Deleting, moving, or rolling back published tags.

## Decisions

### Derive release intent from Conventional Commits

release-please attributes a commit to a module by the files it touches and picks
the bump from the commit type. This removes the class of failure where intent
metadata and the actual diff disagree, and it removes a required extra file from
every release-worthy pull request.

The cost is that an unparsable or mistyped subject silently produces no release.
That cost is mitigated by a required CI gate: every non-merge commit in a pull
request must be a Conventional Commit.

### Register modules explicitly rather than by discovery

`release-please-config.json` names all ten published modules. Discovery is not
used because `examples/` and `test/` contain modules that must never be
published, and because the tag shape differs between the root module and nested
modules.

`internal/releasecheck` asserts the registry is a bijection with the set of
published `go.mod` files, that each configured tag resolves for the Go tool, and
that no published module carries a local `replace` directive. It is the only
release code the repository owns.

### Exclude nested paths from the root module

The root package receives every commit, so nested module directories are listed
in its `exclude-paths`. A commit that only touches `providers/openai/` therefore
releases only that provider, while a commit spanning core and a provider
releases both.

### Bump nested core requirements after the core tag exists

A release pull request cannot contain the nested `go.mod` bump to the new core
version: that version is not published until the pull request merges, so
`module-resolution` and any `GOWORK=off` build would fail on the release pull
request itself, and `go.sum` could not be updated at all.

The release workflow therefore raises a follow-up
`chore(deps): require core vX.Y.Z in nested modules` pull request after the core
tag is published. `scripts/sync-core-requirement.sh` runs `go get` and
`go mod tidy` per module, so `go.mod` and `go.sum` move together and the
follow-up pull request is verified by the same CI as any other change.

Nested modules pick up the requirement in their own next release. This is
accepted: a nested release always names a core version that is already
published.

### Model the alpha channel as configuration

`versioning: prerelease` with `prerelease: true` and `prerelease-type: alpha`
keeps every release in the `alpha` channel and marks its GitHub Release as a
prerelease. Graduation is a reviewed configuration change (`prerelease: false`),
not a release-time flag, so the current channel is always visible in `main`.

A consequence of the prerelease strategy is that while a `0.1.0-alpha.N` line is
open, a `feat` commit advances the counter rather than the base minor version.
The base version moves when the channel graduates or when a commit carries an
explicit `Release-As:` footer.

### Require a GitHub App token

Pull requests created with the default `GITHUB_TOKEN` do not trigger workflow
runs, so a release pull request would never satisfy the repository's
all-checks-required branch protection. The release workflow mints an app token
instead. This is a one-time repository setup cost.

## Risks / Trade-offs

- **External dependency.** Release behavior is owned by a third-party action and
  its schema. Mitigated by pinning the action by commit SHA and by asserting the
  repository-specific invariants in `internal/releasecheck`.
- **Changelog quality follows commit quality.** Entries are commit subjects, not
  curated release notes. Mitigated by the Conventional Commit CI gate and by
  review of the release pull request before merge.
- **Less direct control over batching.** There is no "release only these
  modules" command; the release contents are whatever has landed. Forcing a
  release for an unchanged module requires a commit that touches it.

## Migration Plan

1. Land the configuration with the manifest recording only the released root
   version, `0.1.0-alpha.1`.
2. Nested modules have never been tagged, so each declares
   `initial-version: 0.1.0-alpha.1`; their first release-worthy commit creates
   the missing tag.
3. Configure the release app credentials before the first release pull request
   is merged.
