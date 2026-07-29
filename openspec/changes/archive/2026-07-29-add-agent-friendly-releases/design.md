## Context

The repository publishes one root Go module and nine provider or middleware
modules from subdirectories. Go requires the root tag to be `vX.Y.Z` and each
submodule tag to be `<directory>/vX.Y.Z`. Submodules depend on the root module,
local development may use `go.work`, and publication must validate the declared
dependency graph without allowing workspace resolution to hide stale versions.

The release workflow must be comfortable for maintainers and coding agents,
must not add a release-framework dependency, and must keep all state changes
reviewable before tags or GitHub Releases are created.

## Goals / Non-Goals

**Goals:**

- Express release intent explicitly in implementation PRs.
- Calculate independent module versions from immutable Git tags.
- Prepare per-module changelogs and internal root requirements in a release PR.
- Publish Go-compatible tags and GitHub Releases safely and idempotently.
- Support prerelease iteration and promotion to stable versions.
- Give agents a low-freedom workflow backed by deterministic commands.
- Use only the Go standard library plus existing `git`, `go`, and `gh`
  executables.

**Non-Goals:**

- Inferring release intent from Conventional Commits or changed paths.
- Building a reusable release framework or plugin system.
- Publishing binaries, containers, or artifacts to a package registry.
- Automating major-version import-path migrations.
- Managing backport branches, multiple Git hosts, or cross-repository releases.
- Deleting, moving, or rolling back published tags.

## Decisions

### Use explicit line-oriented change fragments

Each release-relevant PR adds a Markdown file under `.changes/` with a strict
header:

```text
---
core: minor
providers/openai: patch
---

Add continuation support.
```

The body is the reviewed changelog entry. The command creates and validates the
file, so agents do not need to construct the syntax themselves. This preserves
the useful intent model from Changesets without introducing YAML or JavaScript
dependencies. Path and commit inference were rejected because one cross-module
PR can require different bump levels for different modules.

### Keep a small module registry but derive versions from tags

`release/modules.json` records stable module IDs, directories, module paths,
tag prefixes, changelog paths, initial versions, and internal dependencies. It
does not record current versions. The command discovers current versions from
Git tags, keeping Git as the only publication source of truth.

The registry is explicit rather than discovered dynamically so examples and
test modules cannot accidentally become public. Validation compares registered
public modules with repository `go.mod` files to catch omissions.

### Separate planning, preparation, and publication

The command exposes four operations:

- `change`: create an explicit change fragment.
- `check`: validate registry, fragments, tag shapes, module declarations, and
  publishable `go.mod` replacement policy without mutating files.
- `plan`: aggregate fragments, calculate versions, order dependencies, and
  render text or JSON without mutating files.
- `prepare`: update changelogs and internal requirements, consume fragments,
  and write `release/plan.json` for review in a release PR.
- `publish`: validate the prepared plan and current commit, run release checks,
  and create tags and GitHub Releases. Publication is a dry run unless the
  explicit confirmation flag is present.

Splitting these phases makes ordinary agent work reversible while keeping the
external mutation at a human-controlled boundary.

### Use a prepared plan as the release transaction record

`release/plan.json` records the selected modules, previous and next versions,
tag names, dependency order, changelog entries, and a digest of the consumed
change fragments. `publish` refuses to operate when the plan is invalid,
already superseded, or inconsistent with current tags.

The manifest is overwritten by the next prepared release rather than deleted
after publication. Tags remain authoritative for whether an item was actually
published.

### Support selective planning with partial fragment consumption

`plan` and `prepare` accept repeatable `--module <id>` selectors. With no
selector they retain the default behavior of including all pending release
intent. Every explicitly selected module must have pending intent.

A fragment may name both selected and deferred modules. Preparation consumes
only the selected module entries: it deletes a fully consumed fragment or
rewrites a partially consumed fragment with the deferred bumps and original
summary. The prepared manifest records the expected remainder so publication
can reject lost or altered deferred intent.

Requiring maintainers to split cross-module fragments before a selective
release was rejected because it adds manual syntax editing at the exact
boundary the command is intended to make deterministic.

### Publish dependency roots before dependents

For a release containing core and dependent modules:

1. Validate the repository and core module locally.
2. Create and push the immutable core tag.
3. Test each selected dependent with `GOWORK=off` against the declared root
   version.
4. Create and push the dependent tag.
5. Create or verify the corresponding GitHub Release.

A failure after the core tag leaves the valid core release published and the
failing dependent untagged. Retrying is safe because existing tags must either
point to the current commit or cause a hard failure. A cross-service atomic
transaction was rejected because Git hosting, module proxies, and GitHub
Releases cannot share one transaction.

### Keep the agent skill thin

`.agents/skills/release-ai-sdk/SKILL.md` provides trigger guidance, the safe
command sequence, release-intent rules, and the prohibition on confirmed
publication without explicit user authorization. It contains no release logic;
the Go command remains the only source of behavior and validation.

## Risks / Trade-offs

- **Custom code can grow into a framework** → Keep the supported commands,
  fragment grammar, and GitHub-only publication contract intentionally narrow.
- **SemVer and prerelease edge cases can produce incorrect tags** → Use a strict
  parser, reject unsupported versions, and cover version matrices with
  table-driven tests.
- **A new public module can be omitted from release checks** → Validate all
  non-example, non-test nested module paths against the explicit registry.
- **Local workspaces can hide stale root requirements** → Require
  `GOWORK=off` dependent tests during publication and reject local replacements
  in registered public modules.
- **Publication can partially succeed** → Make tags immutable, publish in
  dependency order, and make every step idempotent.
- **Agents can trigger external mutations too eagerly** → Default publication
  to dry-run and require both an explicit command flag and explicit user
  authorization in the skill.
- **The release command itself can break** → Test planning and publication
  against temporary Git repositories and fake command runners.
- **Selective preparation can lose deferred intent** → Record residual fragment
  state in the prepared plan and validate the exact remainder before
  publication.

## Migration Plan

1. Add the command, module registry, initial changelogs, skill, and validation
   tasks without publishing tags.
2. Bootstrap registry initial versions from the existing root alpha tag and the
   chosen first submodule version.
3. Merge the open module-resolution cleanup so public submodules have real root
   requirements and local development uses `go.work`.
4. Exercise `check`, `plan`, and `prepare` on a synthetic change fragment and
   review the generated release diff.
5. Run a publication dry run before the first explicitly confirmed manual
   workflow dispatch.

Rollback before publication is a normal revert. Published tags are never
rolled back or moved; a bad release is corrected with a new version.

## Resolved Choices

- First submodule releases start at `v0.1.0-alpha.1`, matching the core
  prerelease lifecycle.
- Publication remains manually dispatched and requires an explicit boolean
  confirmation. A future move to automatic publication requires a separate
  design change.
