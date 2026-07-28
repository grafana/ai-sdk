## Context

This repository is a Go port of Vercel's TypeScript AI SDK. `AGENTS.md` already identifies upstream TypeScript behavior as canonical, and `test/conformance` already compares Go UI message chunks and provider request snapshots against TypeScript-generated expectations. The gap is governance: the verified upstream baseline is only implied by TypeScript dependency pins, conformance coverage is not summarized as a compatibility contract, and agent workflows for checking, upgrading, and reviewing parity are not encoded as reusable repo-local skills.

The existing conformance suite should remain the behavioral proof point. This change adds structure around it so contributors can answer three questions quickly: which upstream version are we aligned with, which surfaces are actually covered, and what workflow is required when a change touches parity-sensitive code.

## Goals / Non-Goals

**Goals:**
- Declare a single upstream parity baseline that records the Vercel AI SDK repo/package versions used for verification.
- Document parity coverage across wire format, provider interfaces, provider request conversion, orchestration, structured output, and known gaps.
- Provide explicit commands for checking the current baseline and for upgrading to a new upstream baseline.
- Update agent guidance so parity-sensitive work requires upstream comparison, conformance consideration, and explicit divergence classification.
- Add repo-local Codex skills for scoped parity review and parity upgrade workflows.
- Keep the existing conformance harness as the main automated compatibility signal.

**Non-Goals:**
- Full byte-for-byte compatibility with TypeScript internals where Go idioms intentionally differ.
- Live provider recording in normal CI.
- Immediate conversion of the whole conformance job into a required merge gate.
- Automatic implementation of every upstream feature exposed by the baseline.
- Backward compatibility with undocumented internal workflow conventions.

## Decisions

### Add a First-Class Baseline Manifest

Create a checked-in parity manifest, tentatively `test/conformance/upstream.yaml`, that records the upstream repository, verified commit or tag when known, npm package versions, verification date, coverage status, known intentional deviations, and required verification commands.

This keeps the baseline near the conformance harness that consumes the TypeScript packages, while making it more explicit than `package.json` and the lockfile alone. The manifest should be validated against `test/conformance/tools/package.json` so the declared package versions cannot drift from the packages used to generate snapshots.

Alternative considered: use only `package.json` as the source of truth. Rejected because it cannot explain coverage, known deviations, verification status, or the upstream repo revision behind published beta packages.

### Add a Layered Coverage Map

Add a parity coverage document, either as a section in the manifest or a companion markdown file, that classifies compatibility surfaces by layer, status, confidence source, and gap. At minimum it should cover the core ai-sdk layer, provider contract layer, provider implementation layer, frontend interop layer, and conformance harness layer.

This avoids turning "conformance passes" into an over-broad parity claim. The coverage map makes it clear which areas are protected by tests and which still need upstream source review.

Alternative considered: encode coverage only as OpenSpec requirements. Rejected because contributors need a day-to-day operational map close to the tooling, while OpenSpec remains the durable requirement source.

### Treat Conformance as a Confidence Suite

Use conformance fixtures as the preferred TDD path for bugs and new features
that cross provider or UI wire boundaries. If a reported bug can be represented
as recorded provider chunks, provider request snapshots, or structured output
snapshots, create or update that fixture first and let the failing Go replay
drive the implementation. Provider implementation work should usually be backed
by request snapshots; core orchestration and stream conversion work should
usually be backed by UI chunk or structured output snapshots.

Alternative considered: keep conformance as only a final parity check. Rejected
because the same fixtures are the best executable contracts for behavior that
must remain compatible across upstream upgrades.

### Add Check and Upgrade Commands

Add `mise run parity-check` as the standard local verification entrypoint. It should validate the baseline manifest, typecheck conformance tooling, and run the conformance suite or a documented stable subset. Add `mise run parity-upgrade` as the manual workflow entrypoint that updates tracked TypeScript packages to their latest stable npm `latest` versions and regenerates snapshots.

The upgrade path should be intentionally explicit. Package bumps, snapshot regeneration, changed request snapshots, and implementation fixes should all be reviewable in one change.

Alternative considered: make `mise run test-conformance` the only command. Rejected because parity checking includes metadata consistency and workflow validation beyond replay tests.

### Keep CI Progressive

Keep the existing full conformance job available as signal, but design the tooling so a stable smoke subset can become blocking before the entire suite is promoted. This lets the project raise confidence without making noisy or still-evolving fixtures block unrelated work.

Alternative considered: immediately make all conformance blocking. Rejected because existing CI marks conformance as advisory, which suggests the suite may still produce useful but not-yet-merge-blocking signal.

### Add Repo-Local Parity Skills

Add two skills under `.codex/skills/`:
- `ai-sdk-parity-review`: review a user-defined scope against the registered upstream baseline. The scope may be a PR, current git diff, branch range, package, directory, provider, bug report, or feature area. Findings should focus on problematic parity risks rather than enumerating every harmless difference.
- `ai-sdk-parity-upgrade`: bump the registered upstream baseline, regenerate expectations, run conformance, and turn failures into implementation tasks or documented gaps.

These skills should direct agents to use `mise exec --` for OpenSpec when needed, read the baseline manifest first, and avoid treating upstream TypeScript architecture as something to mirror mechanically in Go.

Alternative considered: put all workflow detail in `AGENTS.md`. Rejected because `AGENTS.md` should stay concise; skills can carry task-specific procedure without bloating the always-loaded guidance.

## Risks / Trade-offs

- [Risk] The manifest becomes stale or duplicates package data incorrectly -> Mitigation: add a validation check that compares manifest package versions with `test/conformance/tools/package.json`.
- [Risk] The coverage map creates a false sense of completeness -> Mitigation: require explicit statuses and known gaps rather than only listing covered areas.
- [Risk] Snapshot churn makes parity upgrades hard to review -> Mitigation: keep `parity-upgrade` manual, require upstream changelog/source review, and separate intentional deviations from implementation bugs.
- [Risk] Full conformance remains advisory too long -> Mitigation: define a smaller stable subset that can become blocking first, then expand the blocking set as fixtures stabilize.
- [Risk] Skills drift from actual commands -> Mitigation: skills should reference `mise run parity-check`, `mise run parity-upgrade`, and the baseline manifest instead of duplicating command internals.
