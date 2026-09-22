## Context

The registered baseline, exact-version references, four TypeScript consumers, provenance checks and independent Go module gate already exist. The selector's pure functions have tests, but its CLI combines online latest selection with file mutation and stamps verifiedAt before validation. The local scheduled job duplicates a weaker ai-only selector, checks out a shared upstream clone and autonomously upgrades. These are workflow defects, not Vercel runtime behavior to port. Reference is the current registered package set; this change must not alter it.

## Goals / Non-Goals

**Goals:** a workflow that generates its own inventory and independently mergeable packages after either daily drift or a long absence; fixed targets across reruns; exact source/maturity/coherence validation; pause/resume; honest completion and publication evidence; safe discovery automation.

**Non-Goals:** performing this upgrade, importing the existing upgrade plan as authority, a new orchestration service, a generic dual-baseline harness, automatic gap acceptance, live recordings, new dependencies or changes to unrelated automations.

## Decisions

1. The upgrade skill owns parity reasoning: establish the comparison, classify findings with a decision matrix, derive mergeable implementation scope and review in both directions. The review skill challenges behavioral correctness and proof. The runbook is a tooling/evidence reference, not a second workflow or agent operating manual. AGENTS and CONTRIBUTING state the invariants and link to those resources. Context is loaded to answer specific findings, not from a blanket checklist. Scheduling, permissions and execution setup remain in local automation instructions. Assessment distinguishes implementation status from evidence; completion distinguishes baseline verification from delivery of the selected capabilities.
2. Extend existing upgrade-baseline.mjs with explicit `select --output=...` and `apply --target=...`; no implicit mutating default. Selection uses the existing mature coherent selector and saves a versioned JSON target with starting baseline identity, selection time, release-age policy, exact versions/publication timestamps and per-package source commits. Creation refuses overwrite. Selection does not mutate pins or change upstream checkouts.
3. Apply validates the entire target and all consumer manifests before writing. Exact-version metadata/tag lookups recheck timestamps, coherence and source identity without querying latest. A changed starting baseline or age policy requires reassessment. Reapplying the same target is allowed only when the entire current version set is either its source or target; unrelated/mixed baselines are rejected. Consumer pins must likewise belong to the corresponding baseline. This permits resuming generation/validation without reselection.
4. Applying a new target clears verifiedAt instead of inventing successful verification. Baseline validation rejects missing/invalid verification dates. The runbook requires running the underlying checks and reviewing attestation before a maintainer records the verification date, then running the complete gate again. The tool does not claim it can mechanically attest semantic review. Reapplying a target does not erase an already recorded date or unrelated gap metadata.
5. Preserve the selected target with the assessment; it is a small reproducible input, not a raw candidate manifest/log bundle. Its location does not require a persistent active OpenSpec change. Treat it as reviewable data, not approval of scope. Later releases do not alter the comparison; deliberately changing the target or using intermediate checkpoints requires reassessing the affected decisions.
6. Local scheduled automation becomes discovery-only and delegates to the checked-out repo skill/runbook. It reports newer releases without applying targets, changing source checkouts, approving gaps, committing or posting. If new tooling is not present on main it stops safely. Local prompts route parity work through repo-owned guidance; global generic development instructions need no project-specific duplication.

## Risks / Trade-offs

- Frozen metadata can become unavailable or tags move → apply rechecks exact sources and stops; never falls back to latest.
- Source fixture tests miss unsupported behavior → source/test inventory is mandatory; green replay alone cannot declare full parity.
- Workspace tests hide unpublished dependencies → every package names producer/consumer revisions and runs the existing fresh-cache module gate early.
- A saved record can be edited → validate its fields and live exact-version evidence; review records/target identity, not a misleading claim of cryptographic approval.
- Local configuration is outside Git → back it up outside the repo, validate and dry-run without launch; document the change and deployment dependency on merged repo tooling.

## Migration Plan

Create and test the CLI boundary, then align docs/skills and local automation. Existing bare parity-upgrade calls fail with actionable usage instead of silently selecting. Preserve baseline pins, snapshots and existing feature work. The next approved upgrade assessment derives its own scope from source/tests and Go behavior; earlier research is a later completeness cross-check. This change does not execute an upstream upgrade or launch automation.
