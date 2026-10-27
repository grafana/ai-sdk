# Proposal

## Why

One logical library release never converges. Every module has its own release
pull request, and readiness requires each pinned prerequisite tag to contain the
prerequisite's current source on `main`. A dependent can therefore release only
after its prerequisites are tagged and Renovate has repinned them, which takes a
reviewed pull request and a forced three-day hold per level. Each merge to
prerequisite source in the meantime makes the dependent stale again, so Bedrock
and Azure have never released (#245).

The freshness rule adds little evidence. At a release commit, readiness already
tests each library standalone against its pinned prerequisites, and workspace CI
tests it against the prerequisite source being tagged in the same release.

## What Changes

- Release-please proposes all pending libraries in one release pull request. Each
  library keeps its own version, changelog, tag and GitHub Release. Gateway keeps
  its own release pull request.
- Readiness and post-merge authorization accept a release pull request that
  changes several library versions, and validate every released library.
- Readiness no longer requires a pinned prerequisite to contain the
  prerequisite's current source. Prerequisites must still be canonical tags that
  pass the standalone and consumer checks.
- Publication moves a release pull request's lifecycle labels only after all of
  its libraries are published.
- First-party requirement updates stay with the grouped Renovate pull request;
  they are no longer on the release path.

## Capabilities

### New Capabilities

### Modified Capabilities

- `multi-module-release-management`: one combined library release pull request,
  multi-component readiness and authorization, no prerequisite freshness,
  per-pull-request lifecycle labels.
- `module-validation-modes`: remove prerequisite source ownership snapshots,
  which existed only for the freshness rule.

## Impact

- `release-please-config.json`, `scripts/release_readiness.py`,
  `scripts/release_authorization.py`, `release/tooling` preparation and
  publication, `internal/releasecheck`, release documentation.
- Released `go.mod` files may declare older first-party minimum versions than
  the libraries released alongside them.
- A library whose source needs an unreleased prerequisite API fails readiness and
  blocks the combined pull request. Marking that package
  `separate-pull-requests: true` moves it to its own release pull request until
  its prerequisite is released and repinned.
