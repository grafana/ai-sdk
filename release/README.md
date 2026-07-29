# Release runbook

The repository publishes the core SDK, each provider, and each middleware
integration as independently versioned Go modules. The release command uses
only the Go standard library plus the existing `git`, `go`, and `gh`
executables.

Current module IDs and tag prefixes live in
[`modules.json`](modules.json). Root releases use `vX.Y.Z`; nested releases use
`<module-directory>/vX.Y.Z`.

## Record release intent in normal pull requests

Every public API, behavior, compatibility, dependency, or user-visible bug
change adds a reviewed fragment:

```bash
mise run release -- change \
  --name continuation-support \
  --summary "Add continuation support to streamed responses." \
  --bump core=minor \
  --bump providers/openai=patch
```

Use `patch` for compatible fixes, `minor` for compatible features, and `major`
for incompatible changes. Tests, CI, internal refactors, tooling, and
documentation-only changes do not need release intent unless they change a
published module's behavior.

Validate pending intent without modifying files:

```bash
mise run release -- check
mise run release -- plan
```

## Plan all pending modules

With no selector, planning and preparation include every module with pending
intent:

```bash
mise run release -- plan
mise run release -- prepare
```

Run `prepare` only after reviewing the plan. It updates selected changelogs and
internal core requirements, consumes selected intent, and writes
`release/plan.json`. Commit those changes in a release pull request.

## Release only named modules

Pass repeatable module selectors to release a subset:

```bash
mise run release -- plan \
  --module providers/openai \
  --module middleware/logger

mise run release -- prepare \
  --module providers/openai \
  --module middleware/logger
```

Use the exact same selectors and prerelease flags for `plan` and `prepare`.
Every selected module must have pending intent.

If a fragment names selected and deferred modules, preparation consumes only
the selected entries. It rewrites the fragment with the deferred bumps and its
original summary. For example, selecting only `providers/openai` changes:

```text
core: minor
providers/openai: patch
```

into:

```text
core: minor
```

The prepared manifest records this remainder, and publication stops if the
deferred fragment is later lost or altered.

## Manage prereleases

Existing prerelease channels continue automatically:

```bash
mise run release -- plan
```

For example, a patch after `v0.2.0-alpha.1` becomes
`v0.2.0-alpha.2`. Start or switch channels explicitly:

```bash
mise run release -- plan --prerelease beta
mise run release -- prepare --prerelease beta
```

Promote prereleases to their stable base intentionally:

```bash
mise run release -- plan --stable
mise run release -- prepare --stable
```

Selectors can be combined with either option.

## Publish a prepared release

After the release pull request merges, preview publication:

```bash
mise run release -- publish
```

This is always a dry run. It reports tests, tags, and GitHub Releases without
changing external state.

Prefer the manually dispatched `Publish Go module releases` GitHub workflow for
confirmed publication. It requires an explicit boolean confirmation and only
runs from `main`. Local confirmed publication is available for an explicitly
authorized recovery:

```bash
mise run release -- publish --confirm
```

Core is tagged before selected dependents are tested with `GOWORK=off`.
Publication retries are safe: tags at the prepared commit are reused and pushed
again, while tags at another commit stop the release. Never move or delete a
published tag; correct a bad release with a new version.

## Add a public module

Add its `go.mod`, initial `CHANGELOG.md`, and explicit entry to
[`modules.json`](modules.json). Nested module IDs and tag prefixes match their
repository directory. Registered public modules must not contain local
filesystem `replace` directives.
