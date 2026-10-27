# Design

## Context

See proposal.md for motivation. Current pipeline, as observed on `main`:

- `release-please-config.json` sets `separate-pull-requests: true`. The release
  workflow (`publish.mjs`) publishes authorized merged candidates, then
  `prepareLibraries` (`activation.mjs`) builds one release-please manifest per
  component and asserts each candidate changes only the manifest and that
  component's changelog.
- `scripts/release_readiness.py` requires exactly one changed component version,
  a generated footprint of manifest + changelog, and prerequisites that are
  tagged on the public proxy with owned source equal to the candidate base.
  `go.mod` and `go.sum` are owned source.
- `merged-candidates.mjs`, `authorize.mjs` and `publication.mjs` treat several
  candidates from one pull request as ambiguous.
- `module-policy.sh` `pins` and `standalone` force `GOPROXY=https://proxy.golang.org`;
  `boundary` rejects `replace` directives in root `go.work`.
- Root `go.work` tolerates untagged first-party versions only because
  `test/conformance`, `test/integration/testserver` and the examples replace some
  first-party modules with local paths. `go.gateway.work` has no such module and
  fails on an untagged version.

Spike evidence (local, nothing published), collected before writing this change:

- Module archives built from a tagged tree with `golang.org/x/mod/zip` semantics
  (plus the root `LICENSE` that the Go command adds to nested modules lacking one)
  reproduce the public proxy's `h1:` and `go.mod` hashes for root `v0.1.0-alpha.3`
  and three nested `v0.1.0-alpha.1` tags.
- A simulated release of all 11 libraries on one commit, with `go.sum` filled in
  dependency order from a file `GOPROXY`, produced 38 first-party checksums that all
  match archives rebuilt from the final commit. All libraries passed `go mod verify`
  and readonly standalone builds from a fresh cache, Bedrock and Azure tests passed,
  and an external consumer resolved and built the set with a matching `go.sum`.
- A reused fake version picked up a stale archive from a shared module cache,
  producing a mismatch. Computation therefore needs an isolated cache.
- `separate-pull-requests: false` (in memory) produced one `chore: release main`
  pull request for all pending components; `buildReleases` on it produced every
  per-component tag. A per-package `separate-pull-requests: true` kept Gateway in
  its own pull request.
- A non-published module whose `go.mod` replaces every first-party library with its
  directory, added to both workspace files, made root workspace builds and the
  Gateway plugin backend build (`-mod=readonly -trimpath`) succeed with untagged
  requirements and no network lookups.

## Goals / Non-Goals

**Goals:**

- One approval per library release round, independent of how many modules release.
- Co-released prerequisites are fresh by construction; no follow-up bump pull requests.
- Recorded checksums are proven equal to what the public proxy will serve.
- Keep the existing guarantees: App identity, signed generated commits, exact
  candidate tree, frozen inventory, immutable tags and fail-closed checks.

**Non-Goals:**

- Gateway publication (#459) and Gateway workspace release attribution.
- Lockstep versions across modules.
- Changing release intent derivation or the prerelease channel.
- Merge queue adoption or changes to organization rulesets.

## Decisions

### D1. Combine libraries with release-please configuration, not a custom version engine

Set `separate-pull-requests: false` at the top level and `true` for `ai-gateway`.
release-please keeps per-component versions, changelogs and tags.

- Alternatives: `linked-versions` (lockstep; releases unchanged modules), relaxing
  freshness (publishes untested combinations), an in-house bump PR after each tag
  (still serial, still re-approval per hop), a release-please workspace plugin that
  co-releases every dependent (releases modules with no changes).

### D2. Update requirements only in released modules and Gateway

`go.mod`/`go.sum` are owned source for freshness. Bumping a module that is not
released would change its owned source without a tag, making it a stale
prerequisite for its own dependents. Released modules require each prerequisite's
co-released version, otherwise its latest tag. Gateway always follows, because its
plugin builds from the workspace and its pins are never published.

- A released module whose non-released prerequisite has unreleased owned source
  stays blocked by the existing freshness rule; the remedy is a release-worthy
  change to that prerequisite.

### D3. Generate requirement files from candidate archives

A small Go command in a non-published module at `release/candidate` (keeping
`golang.org/x/mod` out of the root module's requirements; classified as local-only
like `test/*` by module inventory and publishable-module validation) takes a
candidate tree and the released component versions and:

1. rewrites first-party requirements per D2;
2. orders modules by first-party dependency level;
3. for each level, builds archives of the level's released modules into a file
   proxy, then runs `go mod tidy` for the next level with
   `GOPROXY=file://<candidate>,https://proxy.golang.org`, `GONOSUMDB` for the
   repository's modules, `GOWORK=off` and an isolated `GOMODCACHE`;
4. emits the changed `go.mod`/`go.sum` files and the archive hashes.

Preparation in the release workflow materializes the candidate tree (current `main`
plus release-please's generated updates), runs the command, and appends its output
to the release-please candidate before `createOrUpdatePullRequest`, so the App still
creates one signed commit.

- Readiness and authorization rerun the same command on the tree under test and
  require byte-identical generated files. This rejects hand edits and unexpected
  third-party changes without a separate allowlist of versions.
- Alternative: let `tidy` contact the public proxy for co-released versions after a
  first publication; rejected because it reintroduces a two-step release.

### D4. Resolve unpublished versions through a workspace module, not `replace` in workspace files

Add `test/workspace` (local-only like other `test/*` modules) whose `go.mod`
replaces every published first-party library with its directory, and include it in
`go.work` and `go.gateway.work`. Go applies a workspace module's replacements to
the whole workspace.

- Structural checks require the module to list exactly the published libraries
  with in-repository targets, and continue to reject `replace` in workspace files.
- `verify-gateway-workspace` accepts the published modules plus this module.
- Alternatives: version-specific `replace` lines in workspace files (conflicts with
  the structural rule and changes every release), relying on existing test-module
  replacements (incomplete: Azure, Agent Observability, Enrichment and Prometheus
  are not covered; Gateway workspace has none).

### D5. Verify co-released prerequisites against candidate archives

Readiness (`release_readiness.py`, `release_consumer.py`) and authorization build
candidate archives from the tree under test: the PR head merged with current base
for readiness, the merge revision for authorization. Co-released prerequisites
resolve only from those archives and must match recorded checksums; already-tagged
prerequisites keep the public-proxy origin, tag-target and ancestry checks.
`module-policy.sh standalone` gains an explicit candidate mode used only by
readiness; its on-demand public-proxy behavior is unchanged.

The merged-pin check accepts an untagged first-party version only when it equals
that module's version in the candidate manifest and its candidate archive matches
the recorded checksum. On ordinary pull requests manifest versions are already
tagged, so the rule needs no release-PR detection. A non-App pull request that edits
the manifest is already rejected by the required `release-readiness` check.

### D6. Per-component records from one pull request, dependency-closed publication

Discovery yields one record per released component of the merged combined pull
request; ambiguity is keyed by component and tag, not pull request number.
Authorization computes each candidate's co-released prerequisites from the merge
revision's manifests and blocks it when any of them is blocked. Publication tags
prerequisites before dependents at the single merge commit, then fetches each new
version from the public proxy and compares hashes with bounded retries. Retries
resume from canonical tag state; existing tags are never moved.

- The public proxy must not be queried for a version before its tag exists, because
  it caches the miss. Verification runs only after tag creation.

### D7. Remove first-party Renovate updates

Delete the tagged-prerequisite rule and its lookup rule, and extend the
first-party disable rule to every update type. Third-party grouping is unchanged.

## Risks / Trade-offs

- [Merged tree differs from the validated head] → Squash merge of an up-to-date
  branch keeps the tree; authorization already requires the merged tree to equal
  the validated candidate tree, and post-publication verification catches any
  residual mismatch before release notes claim success.
- [Go changes module archive rules] → The generator pins `golang.org/x/mod`, and
  post-publication verification fails loudly; the fidelity test against existing
  tags runs in CI so drift is caught before a release.
- [One failing library delays others in the same pull request] → Dependency-closed
  authorization publishes every candidate whose closure passes.
- [Larger release pull request is regenerated on every relevant main push] →
  Generation is deterministic and incremental per level; reviewers re-approve once
  per regeneration, which was already true per module.
- [Root release changes many modules' pins only when they also release] → Pins of
  non-released modules lag until they release; workspace builds are unaffected.
- [Gateway stays separate] → Gateway pins are updated by the library pull request,
  so Gateway no longer needs Renovate either; folding Gateway in is revisited after #459.

## Migration Plan

1. Land tooling, workspace module, structural checks and docs behind the existing
   `RELEASE_LIBRARY_MODE` gate, with tests using recorded fixtures; keep the
   configuration change for last.
2. Flip `separate-pull-requests` and remove the Renovate first-party rules in one
   pull request. The next release run closes superseded per-module release pull
   requests and opens the combined one; existing tags, releases and manifest
   versions stay as they are.
3. First live round: review the combined pull request's generated requirement
   files, merge it, and record tag, proxy-hash and consumer evidence on #245/#468.
4. Rollback: restore `separate-pull-requests: true` and the Renovate rule. Published
   tags remain valid; nothing is retagged.
