---
name: ai-sdk-parity-upgrade
description: Discover, assess, resume or implement an incremental Vercel AI SDK parity upgrade. Use for upstream drift, routine or catch-up upgrades, frozen-target selection, baseline transitions and capability rollout planning. No existing upgrade plan is required.
---

# AI SDK Parity Upgrade

Follow [the operational runbook](../../../test/conformance/UPGRADING.md). It owns
policy; do not invent a separate latest-version rule or merge strategy here.

## Entry and authority

Determine whether the request is **discover**, **assess/plan**, **resume**, or
**implement an approved package**. Default to assessment, not mutation, when unclear.
Scheduled discovery never implements, accepts gaps or writes to GitHub. Delegation
and GitHub actions require the applicable operator authorization.

Read `AGENTS.md`, `test/conformance/upstream.yaml`, `PARITY.md`, the conformance
README/runbook, active OpenSpec changes, relevant open PRs and current `main`.
Find an active transition even if its target is older than today's latest. Memory
and local checkout HEAD are not source or approval authority.

## Discover and freeze

1. Resume an existing target when applicable. Do not replace it on a daily run.
2. For a new cycle, use `TARGET=/absolute/new-target.json mise run parity-select`.
   The command saves an immutable-path record and leaves canonical pins unchanged.
3. Compare the complete coherent set, not just the `ai` version. If unchanged,
   report no baseline update; existing capability gaps remain a separate concern.
4. Inspect exact per-package tags/commits, installed sources or matching raw GitHub
   sources. Verify package manifests. Never switch a shared upstream checkout.
5. New releases become discovery information for a later cycle. Changing this
   target requires approval, a new record and incremental reassessment.

## Assess and propose without a prewritten plan

Read upstream implementation and tests across the selected range, including changes
not represented by current fixtures. Cover supported core/provider/frontend/Gateway
surfaces and the harness. Do not interpret every upstream product as in scope.

Produce the runbook's compact delta inventory: observable change, exact references,
Go implementation status, evidence/coverage, disposition and owner. Distinguish
missing behavior from missing proof. Do not call unresolved decisions accepted gaps.

Propose coherent packages with explicit outcome, exclusions, dependencies and tests.
Run the publication assessment early: root/provider/module changes may require a
merged publicly resolvable producer before its consumer. Workspace green is not
enough. Every PR must be independently green; no final cumulative merge.

Use existing OpenSpec artifacts for target, assessment, decisions and tasks. Do not
create an elaborate parallel tracking system or commit raw research. Obtain scope,
API and gap approval before implementation. Headless runs report blockers instead
of guessing approval. An old plan is a later completeness cross-check, not input
that replaces discovery.

## Implement the approved package

- Use a current-main worktree and one writer. Reuse prior work only after matching
  its scope and source version; do not wholesale-copy completed spec claims.
- For the baseline transition, use the approved record with `mise run parity-apply`
  or `mise run parity-upgrade`. No bare implicit latest upgrade is supported.
- Implement behavior and regression assets together. Follow conformance-first TDD
  and immutable-recording/exact-upstream provenance rules.
- Include canonical pins, lockfile, expectations and reviewed Gateway attestation
  in the same green transition. Additive predecessors must pass the old baseline.
- Record accepted durable gaps in the coverage contract without weakening tests.
  Unsupported or untested behavior cannot become parity by changing a label.

## Verify, resume and finish

Apply clears the old verification date; it does not certify the new target. Run
underlying checks and review evidence, record the actual verification date only
then, and rerun complete required gates. Include fresh-cache module resolution,
Gateway boundary/runtime/privacy, frontend, conformance and source inspection.
Check exact deployed/module versions before claiming consumer adoption.

On pause/resume, read the frozen record, decisions and package status. Reassess
changed baselines, module graphs or scope; do not reselect because time passed.
Keep raw logs/probes externally and concise evidence with the change.

Report separately: **baseline promoted** and **approved rollout complete**. Every
approved capability needs verified implementation or an explicitly revised scope.
A later cycle may start before feature backlog is exhausted only after affected
work is reassessed. Commit/push/PR creation and automation execution are separate
permissions, not automatic consequences of this skill.
