---
name: release-ai-sdk
description: Plan, prepare, validate, or publish independent releases for the AI SDK core, provider, and middleware Go modules. Use for release intent, change fragments, changelogs, semantic versions, prereleases, Go module tags, GitHub Releases, or when a public SDK change needs a release decision.
---

# Release AI SDK

Use the repository release command as the source of truth. Do not calculate
versions, compose tags, edit generated changelog sections, or modify
`release/plan.json` by hand.

## Decide Release Intent

Add a change fragment when work changes a public module's API, behavior, wire
compatibility, dependencies, or user-visible bug behavior. Select bumps
independently:

- `patch`: backward-compatible fix or small behavior correction.
- `minor`: backward-compatible feature or public API addition.
- `major`: incompatible public API or behavior change.

For tests, CI, internal refactors, tooling, or documentation-only changes that
do not alter a published module, do not invent a bump. State explicitly in the
work summary that no release fragment is needed.

Create release intent through the command:

```bash
mise run release -- change \
  --name continuation-support \
  --summary "Add continuation support to streamed responses." \
  --bump core=minor \
  --bump providers/openai=patch
```

Use one concise user-facing summary. Add every affected public module to the
same fragment when the change crosses module boundaries.

## Validate and Preview

Run validation after creating or editing release intent:

```bash
mise run release -- check
mise run release -- plan
```

Use `plan --json` when exact machine-readable values are useful. Existing
prerelease channels continue automatically. Use `plan --prerelease beta` to
start or switch a channel, and use `plan --stable` only for an intentional
promotion to the stable base version.

When the user names a release subset, pass repeatable selectors:

```bash
mise run release -- plan \
  --module providers/openai \
  --module middleware/logger
```

Report both the selected releases and intent that will remain pending. With no
selector, the plan includes all pending modules.

Report the command-calculated modules, versions, and tags. If validation finds
a local `replace`, stale module path, unknown module, or inconsistent tag,
resolve that repository issue instead of bypassing the check.

## Prepare a Release

Only prepare when the user asks to create the release changes:

```bash
mise run release -- prepare
```

Pass the exact same module selectors and prerelease or stable flag used for the
reviewed plan. Preparation updates per-module changelogs and internal root
requirements, consumes selected fragment entries, preserves deferred entries,
and writes `release/plan.json`. Review the complete diff, including remaining
`.changes/` files, and rerun focused tests. Preparation never creates tags or
GitHub Releases.

## Publish Safely

Preview publication without external mutation:

```bash
mise run release -- publish
```

Do not pass `--confirm` unless the user explicitly asks to publish the prepared
release in the current task. A request to prepare, version, update changelogs,
or make a release PR is not publication authorization.

Before confirmed publication:

1. Verify the worktree is clean and `release/plan.json` is the reviewed plan.
2. Run the dry run and report the exact tags and GitHub Releases it will create.
3. Confirm the user asked for those external effects.
4. Prefer the manually dispatched release workflow. If publishing locally is
   explicitly requested, run `mise run release -- publish --confirm`.

Never move or delete an existing tag. A partial release is retried with the
same prepared plan; the command accepts tags already at the prepared commit and
stops on tags at any other commit.
