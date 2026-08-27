## Why

The repository contains ten independently versioned public Go modules but has
no process for turning merged work into versions, changelogs, path-prefixed
tags, and GitHub Releases. Contributors already write Conventional Commits and
CI already merges through pull requests, so the release contract can be derived
from information the repository produces anyway instead of from a second,
hand-maintained intent format.

## What Changes

- Adopt release-please in manifest mode as the release system for the root
  module, the five providers, and the four middleware modules.
- Register every published module with a Go-compatible tag shape: `vX.Y.Z` for
  the root module and `<module-directory>/vX.Y.Z` for nested modules.
- Derive each module's release from the Conventional Commits that touch its
  files, and keep unrelated modules out of a release by excluding nested paths
  from the root module.
- Publish through a continuously groomed release pull request: pushes to `main`
  refresh it, and merging it creates every tag and GitHub Release.
- Let Renovate repoint nested modules at a released core version once the core
  tag exists, so `go.mod` and `go.sum` are updated together and module
  resolution stays verifiable in CI.
- Add CI validation that every published module is registered, that its tag
  shape resolves for the Go tool, and that it has no local `replace` directive.
- Add a CI gate that every commit in a pull request is a Conventional Commit,
  because an unparsable subject silently drops a change from its release.
- Add a maintainer runbook and a repository-local agent skill for release
  intent.

## Capabilities

### New Capabilities

- `multi-module-release-management`: Commit-derived release intent, independent
  version calculation, changelog generation, Go-compatible tagging, and
  automated publication for public Go modules.
- `agent-release-workflow`: Repository-local agent instructions for expressing
  release intent and for staying out of publication.

### Modified Capabilities

None.

## Impact

- Adds `release-please-config.json`, `.release-please-manifest.json`, a release
  runbook, and a per-module changelog surface owned by release-please.
- Adds a release workflow, a Conventional Commit workflow, and one CI step in
  the existing build job.
- Adds a small standard-library-only validation package under `internal/`.
- Adds a repository-local skill under `.agents/skills/`.
- Requires a GitHub App token so the release pull request can trigger the
  required CI checks.
- Does not change SDK runtime behavior or the upstream parity baseline.
