## Why

The repository contains independently versioned Go modules but has no reliable,
reviewable process for declaring release intent, updating per-module changelogs,
coordinating root dependency versions, or creating the required path-prefixed
tags. A repository-owned workflow can keep this narrow release contract
dependency-free while making it equally usable by maintainers and coding agents.

## What Changes

- Add a standard-library-only Go release command with explicit change fragments,
  module registry validation, release planning, release preparation, and safe
  publication support.
- Add independent changelogs and Go-compatible tag planning for the root module,
  providers, and middleware modules.
- Allow maintainers to plan and prepare only named modules while preserving
  unreleased intent from shared cross-module fragments.
- Treat Git tags as the authoritative current-version source and record the
  prepared release batch in a reviewable manifest.
- Coordinate root and dependent module releases so a submodule is validated
  against its declared published root version before its tag is created.
- Add CI validation for release metadata and publishable module configuration.
- Add a concise repository-local skill that instructs agents to create release
  intent, inspect plans, prepare release changes, and avoid unsafe publication.
- Add a co-located maintainer runbook for the release command.
- Exclude examples and test-only modules from release publication.

## Capabilities

### New Capabilities

- `multi-module-release-management`: Explicit release intent, independent
  version calculation, changelog preparation, dependency ordering, validation,
  and idempotent publication for public Go modules.
- `agent-release-workflow`: Repository-local agent instructions and commands for
  participating safely in the release process.

### Modified Capabilities

None.

## Impact

- Adds repository tooling under `internal/cmd/` and release metadata under a
  dedicated top-level directory.
- Adds per-module changelog files for the root, providers, and middleware.
- Adds `mise` tasks and CI checks for release intent and module registry
  validation.
- Adds a repository-local skill under `.agents/skills/`.
- Uses existing `git`, `go`, and GitHub CLI capabilities at publication time;
  it adds no Go library dependency and does not change SDK runtime behavior or
  the upstream parity baseline.
