# Design

## Context

Each library is a separate Go module with its own release-please package and
`<module>/vX.Y.Z` tag. Builds inside the repository use `go.work` (SDK) or
`go.gateway.work` (Gateway) so that coordinated changes compile against local
source, while each `go.mod` must require published first-party versions. Release
readiness then proves each library against those published versions through the
public proxy and requires them to contain the prerequisite's current source.

A library that uses unreleased prerequisite behavior can therefore only release
after the prerequisite is tagged and a requirement update has merged. Requiring
the co-released version inside the release pull request instead fails for two
reasons: the version does not exist yet, and `go.sum` needs its checksum.

Local `replace` directives remove both problems. Go applies them only in the main
module ("replace directives only apply in the main module's go.mod file and are
ignored in other modules", Go modules reference), and a module replaced by a
local directory needs no checksum. A spike on current `main` confirmed that
bedrock, which cannot build against any published root, builds, vets and tests
with `GOWORK=off` and `GOPROXY=off` against untagged requirements, with no
first-party `go.sum` entries.

## Goals / Non-Goals

**Goals:** independent module versions released only on change, no
pseudo-versions, no release-age wait, one release pull request without manual
steps.

**Non-Goals:** replacing the custom release tooling with the stock release-please
action, Gateway plugin publication (#459), and changing third-party dependency
management.

## Decisions

### D1. Local replacements in every module

Every tracked `go.mod` replaces each first-party module in its build graph,
including transitive ones, with its relative directory. Go ignores replacements
of dependencies, so bedrock replaces root as well as openai. A module-policy
check requires the set to be complete and points only at registered module
directories. Published SDK modules never require or replace Gateway.

Alternatives: keeping workspaces only (modules cannot build alone against
untagged versions); generating checksums for untagged archives (#553; large
tooling); sequential releases (do not converge).

### D2. No workspaces

`go.work`, `go.gateway.work` and their sum files are removed. Every task and CI
job runs inside a module with `GOWORK=off`. One mechanism means CI builds exactly
the published module plus local paths; a workspace could hide a missing
replacement. Recent gopls supports repositories with several modules; a
developer may create an uncommitted `go.work`, which `.gitignore` excludes and
which does not change first-party resolution.

### D3. Requirements equal the version manifest

Every first-party requirement in every tracked `go.mod`, including Gateway,
examples and test modules, equals the manifest version of that module, or its
registered initial version if it has never been released. CI enforces this on
every pull request, so pseudo-versions and stale requirements cannot merge. A
source pull request that adds a first-party dependency adds it at that version.

### D4. Requirement updates in release preparation

release-please has no Go workspace plugin, and its version markers cannot target
a specific package on a line, so preparation edits the requirement lines itself.
After release-please computes the candidate, preparation sets every first-party
requirement to the candidate manifest versions and adds those files to the same
App-signed commit. No `go.sum` changes are needed. Modules that are not released
also get updated requirements; that changes no release, because release commits
are not release intent.

### D5. Readiness and authorization

The release pull request's normal CI builds and tests every module with
same-revision source, which is the behavior being tagged. Readiness keeps release
identity, metadata, changelog and footprint checks, and additionally requires
that the only `go.mod` changes are first-party requirement lines set to the
candidate manifest versions, and that every first-party requirement of a
released library is an existing tag or released by the same pull request.
Resolving prerequisites through the public proxy, the pre-merge consumer proxy
and prerequisite freshness are removed. Post-merge authorization keeps its
merge identity, tree and evidence binding.

### D6. Post-publication verification

After a library is tagged, publication builds a fresh external consumer of the
new version through the public Go proxy and checksum database, with no
replacements, and runs its consumer scenarios. It queries the proxy only after
the tag exists, so no missing version is cached. A failure is reported and fixed
forward with a new release; tags are never moved.

### D7. Non-releasing source changes

Local replacements test the dependent against current prerequisite source. If a
prerequisite changed after its last tag through a non-releasing commit, the
dependent's release could require a tag without that change. `refactor` becomes a
releasing type, which covers source changes under the commit convention, and D6
catches mislabelled changes after publication.

### D8. Dependency-closed publication

A released library is tagged only when every co-released prerequisite was tagged
in the same run, transitively; otherwise it is blocked with the prerequisite as
reason and retried by the next run. Lifecycle labels move once all libraries of
the pull request are published.

### D9. Gateway boundary

`ai-gateway/go.mod` replaces the SDK modules it uses and builds the plugin
backend directly. The AGPL boundary check keeps rejecting Gateway requirements,
replacements and imports outside `ai-gateway/`. The Gateway-free isolation check
copies the repository without `ai-gateway/` and builds and tests root and the
Grafana client per module.

### D10. One pull request

Current readiness rejects local replacements, so landing the module change
before the release change would block releases. One pull request with one commit
per area (module layout, validation, release flow, documentation) avoids that
intermediate state.

## Risks / Trade-offs

- **Non-releasing changes (D7):** a mislabelled change can still publish a
  dependent whose requirement lacks it; D6 detects it after publication.
- **Replacement completeness:** a missing transitive replacement breaks only the
  in-repository build of that module; the D1 check reports it.
- **Renovate third-party updates:** without a workspace, a module that lists a
  dependency must list the version its local first-party replacements select, or
  readonly builds fail with "updates to go.mod needed". Renovate disables indirect
  gomod updates by default, so the repository enables them; every `go.mod` listing
  a dependency then moves in the same grouped branch, and the base preset's
  `gomodTidy` tidies each one. A failure surfaces in CI with the instruction to
  run `mise run tidy`; the first grouped Renovate run confirms this behavior.
- **Editor experience:** cross-module references may be less complete without a
  workspace.

## Migration Plan

One pull request adds replacements, sets requirements to the manifest
(replacing current pseudo-versions in Azure, Gateway and test modules), removes
the workspaces, and switches release preparation, readiness, publication,
release-please configuration and Renovate. After merge, close open per-module
release pull requests and the grouped first-party Renovate pull request; the next
release run proposes one pull request for all pending libraries. Merged release pull
requests must be published before this change lands, because their readiness
evidence predates the combined configuration. Rollback reverts
the pull request; published tags stay valid.
