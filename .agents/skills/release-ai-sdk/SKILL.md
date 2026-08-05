---
name: release-ai-sdk
description: Decide and describe release intent for the AI SDK core, provider, and middleware Go modules. Use for Conventional Commit messages that drive releases, changelogs, semantic versions, prereleases, Go module tags, or when asked what would be released next.
---

# Release AI SDK

Releases are produced by release-please from merged Conventional Commits. Never
calculate a version, edit a generated changelog section, write a tag, or edit
`.release-please-manifest.json` by hand.

## Express Release Intent in the Commit Message

The commit subject is the release intent. Choose the type from the effect on the
published module, not from the size of the diff:

- `fix:` or `perf:` for a backward-compatible fix or behavior correction.
- `feat:` for a backward-compatible feature or public API addition.
- `feat!:` or a `BREAKING CHANGE:` footer for an incompatible change.
- `chore:`, `ci:`, `docs:`, `refactor:`, `test:`, `build:` for work that does not
  change a published module. These produce no release.

Use a scope that names the affected module when the change is module-specific:

```text
feat(providers/openai): add continuation support to streamed responses
```

A commit is attributed to a module by the files it touches. A change that spans
core and a provider releases both. When work legitimately splits into different
bump levels per module, split it into separate commits rather than picking one
type for everything.

For tests, CI, internal refactors, tooling, or documentation-only changes, use a
non-releasing type and state in the work summary that the change does not
produce a release. Do not invent a version bump.

## Validate and Preview

After adding a module or changing release configuration:

```bash
mise run release-check
```

To report what would be released next from `main`:

```bash
mise run release-preview
```

This is a read-only dry run. It reads the configuration and commit history from
GitHub, so it reflects `main` rather than the working tree. Report the modules,
versions, and tags it prints.
If it reports a package that is not a published module, an unresolvable tag
shape, or a local `replace` directive, fix that repository problem rather than
bypassing the check.

## Do Not Publish

Publication is automatic and belongs to maintainers:

- Every push to `main` refreshes a `chore(main): release Go modules` pull
  request.
- Merging that pull request creates every tag and GitHub Release.

Never create or push a tag, never create a GitHub Release, and never merge the
release pull request on a user's behalf unless they explicitly ask for that
action in the current task. A request to prepare a release, update changelogs,
or bump a version is not authorization to publish.

The full workflow, including prerelease channels, forced releases for unchanged
modules, and adding a new public module, is in
[the release runbook](../../../release/README.md).
