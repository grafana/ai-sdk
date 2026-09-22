# Incremental upstream parity upgrades

This is the operational contract for humans, skills and local automations. It
works for a routine update or catch-up after an absence. A prewritten upgrade
plan is not required: assessment produces the scope and merge sequence.

## Three separate records

- **Registered baseline:** `upstream.yaml`, consumer pins, canonical expectations
  and reviewed evidence on `main`. Normal development compares against it.
- **Active target:** one frozen, coherent package set selected for an upgrade
  cycle. New releases do not invalidate an already mature target.
- **Capability backlog:** explicitly assessed missing behavior and evidence, with
  owners and dispositions. Missing implementation is not the same as missing tests.

Only one baseline transition is active at a time. Its owner and target belong to
the active OpenSpec change, not an agent's memory. Outstanding feature work need
not all finish before another transition, but a new baseline requires reassessing
that work rather than silently comparing it against a moving target.

## 1. Discover and select

Read `upstream.yaml`, `PARITY.md`, current `main`, active OpenSpec changes and
related open PRs. Identify an existing transition even if it targets an older
version than today's latest. Resume or report it; do not automatically replace it.
Inspect the public Go module dependency graph and required CI jobs early.

When starting a new cycle, select the newest coherent stable package set on npm
latest lines satisfying `test/pnpm-workspace.yaml`'s minimum release age:

```bash
TARGET=/absolute/path/to/new-target.json mise run parity-select
```

This writes a new decision record, not canonical pins. It includes the starting
baseline, selection time, maturity policy, exact package versions/publication
values and per-package source commits. Existing output paths are rejected. Use a
persistent operator artifact directory during discovery; after approval the small
record may accompany the OpenSpec change as `target.json`. This record is distinct
from raw logs or generated candidate manifest dumps, which stay outside Git.

Selection does not mean approval, implementation or successful parity verification.
If the selected set equals the registered set, report no baseline update; existing
capability gaps may still warrant work. Review exact package source/tests using
Git objects, dedicated worktrees or installed packages. Never switch a shared
upstream checkout as an automation setup side effect. Verify package manifests;
the `ai` source commit alone does not prove every independently released package.

## 2. Assess before implementation

Trace the entire relevant source/test delta, not only changelog titles or failing
fixtures. Cover core orchestration, provider contract, supported provider adapters,
frontend consumers, Gateway and harness/tooling. Check adjacent behavior and new
public APIs even when no fixture currently exercises them.

Produce a compact inventory in the change's design:

| Item | Evidence | Implementation | Coverage | Disposition / owner |
| --- | --- | --- | --- | --- |
| Concrete behavior and affected surface | Exact source/test references | Present, missing or incorrect | Existing proof and missing proof | Required correction, proposed addition, adaptation, accepted exclusion or unresolved |

Assessment must distinguish:

- Supported-contract corrections that block transition, including known bugs
  without fixtures. Green replay alone cannot make them disappear.
- New capabilities proposed for the cycle or separate feature work.
- Parity-preserving Go adaptations and intentional deviations with rationale.
- Unsupported families and exclusions requiring explicit approval.

Every in-scope upstream change needs a disposition. Unresolved is not accepted.
For each proposed follow-up, identify its observable behavior, regression contract,
owner and whether the baseline can honestly advance before it. An accepted gap
must appear in `upstream.yaml` or `PARITY.md`; it never authorizes skipping an
existing failure, deleting evidence or relaxing a comparator.

The output is a reviewable scope decision, not automatic permission to implement
all new APIs. Obtain operator approval. Headless runs report unresolved decisions
and stop rather than inventing approval.

## 3. Plan independently mergeable packages

Every PR must pass its enforced checks on its own merge candidate. No knowingly
red merge, review-only stack or final cumulative merge that repairs earlier PRs.
Group by coherent behavior and actual dependencies, not line counts or a fixed
number of PRs. One upstream release or option does not automatically deserve a PR.

Common ordering:

1. Additive prerequisites that remain green on the registered baseline.
2. A complete baseline transition: required behavior, coherent pins, lockfile,
   expectations and attestation together.
3. Published consumer adoption where consumers cannot yet reference same-PR changes.
4. Approved capabilities, each with implementation and target regression assets.

For each package record: outcome, exclusions, starting/ending baseline, producer
and consumer modules, publication prerequisites, acceptance commands and handoff.
Run `mise run verify-module-resolution` early: it uses a fresh public cache,
`GOWORK=off`, readonly tests and no replacements in published modules. Workspace
success is not publication proof. Merge the producer, verify its actual merged
revision resolves, then update consumer requirements/sums. Do not invent versions,
pin unmerged work to evade ordering or remove consumer tests to manufacture green.

If one transition is still too large, propose coherent intermediate checkpoints
with rationale and approval. Do not automatically walk every upstream release,
select partial package sets or bypass release maturity.

## 4. Apply the approved target and implement

In the intended transition branch or a disposable investigation worktree:

```bash
TARGET=/absolute/path/to/approved-target.json mise run parity-apply
```

Apply verifies the starting baseline (or an already applied identical target),
current age policy, package inventory, stable/nondecreasing versions, selection-time
maturity, exact publication/source evidence and dependency coherence before writing.
It checks all four consumers before changing any. It queries exact versions, never
latest. Unrelated/mixed baseline or consumer pins fail for reassessment.

To also install and regenerate expectations:

```bash
TARGET=/absolute/path/to/approved-target.json mise run parity-upgrade
```

The upgrade command no longer implicitly selects a target. It updates conformance,
integration, CLI and ProviderWire consumers, the lockfile and expectations using
that target. Reapplication is idempotent; it preserves later gap decisions and an
already recorded verification date. It does not repair a partially edited mixed
baseline: inspect and restore the interrupted edit before retrying.

Follow conformance-first TDD and fixture provenance. Recorded provider inputs are
immutable; upstream imports are byte-identical and indexed. Synthetic transport
failures belong in focused tests or provider-independent UI fixtures. Do not
blindly copy candidate output or older implementation branches.

## 5. Verify and merge the baseline

A new application clears `upstream.verifiedAt`. The date is not the selection date.
The final validation step deliberately fails until verification is completed and
a maintainer records a valid date. Gateway attestation can also fail intentionally
until its differential evidence and mapping are reviewed.

Run the underlying checks, fix implementation differences and review every changed
snapshot, imported source and coverage disposition. Use current `main` and actual
module resolutions, not historical green runs or stale service scope. Privacy tests
must assert the fields being suppressed actually exist in the exercised runtime.

Required evidence includes:

- Fixture inventory/provenance, provider-shape against exact installed source,
  conformance replay and repeat generation for stability.
- ProviderWire schema/client/runtime and Gateway command/privacy/boundary coverage;
  explicitly review `gateway-client-contract.json` rather than waiving its gate.
- Frontend hook scenarios, CLI, focused module/race tests and published-module
  resolution. Label workspace versus deployed/published-consumer evidence.
- Current required CI: formatting, builds, tests, vet/lint, docs/OpenSpec, examples,
  image/security checks and any other enforced jobs.

After successful evidence review, record the actual verification date, then rerun
`mise run parity-check`, `mise run verify-module-resolution`,
`mise run verify-ai-gateway-boundary` and all other required checks on the final
merge state. An incomplete metadata gate is not permission to merge or to skip
underlying tests. No tool stamps a date as a substitute for this review.

## 6. Complete the approved rollout

Track two independent outcomes:

- **Baseline promoted:** the frozen set is registered with green enforced checks
  and honest approved coverage dispositions.
- **Rollout complete:** every capability approved for this cycle is implemented
  and verified, or its scope was explicitly revised. Moving versions alone is not
  completion; an unresolved item cannot be relabeled accepted by omission.

Feature PRs use the registered target and carry their own tests and fixture updates.
Resolve public-module adoption before claiming deployed behavior. Close resolved
gaps and keep remaining owners visible. A new baseline cycle may start before all
feature work finishes only with reassessment of the affected work.

## Pause, resume and new upstream releases

On resume, read the saved target, approved assessment, work-package status, current
baseline and module graph. Re-run checks invalidated by rebases/dependency changes.
Do not rerun selection simply because time passed. Application rechecks the record
against exact live evidence; unavailable sources or moved tags block, not fallback.

Change a target only for an explicit reason, such as security, a serious selected
release defect or a newer fix needed to unblock work. Select a new record, review
the incremental delta, update approval and regenerate/revalidate. Never overwrite
the old target or silently expand the cycle.

## Automation and evidence boundaries

Scheduled automation is discovery-only. It may produce a local candidate/report,
inspect open work and note newer releases. It must not mutate canonical pins or
shared source checkouts, implement, approve gaps, commit, push or create/update PRs.
If this workflow is absent on its checked-out `main`, it stops safely. Local prompts
route into this runbook/skills; they do not implement a second version selector.
Memory is advisory. One rolling discovery report is preferable to daily PR noise.

Execution automation requires separately approved scope and target. Delegation,
GitHub writes and deployment retain their own authorization boundaries.

Keep a concise decision/proof summary with the change. Store raw probes, logs,
full generated diffs and investigation indexes externally. The small approved
target record is an input worth preserving; it is not a signature of approval.
An old upgrade plan may be used after fresh assessment as a completeness cross-check,
never as a substitute for discovering the actual current delta.
