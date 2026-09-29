---
name: ai-sdk-parity-upgrade
description: Upgrade the pinned upstream AI SDK reference, assess the current Go implementation against it, and register parity work for independent implementation afterward.
---

# Pinned-Version Upgrade and Parity Assessment

Separate updating the upstream reference from matching all of its behavior:

1. **Pinned-version upgrade:** update the reference, validate it, assess parity and
   register remaining differences. This produces one independently mergeable PR.
2. **Parity work:** implement the registered behavioral work packages independently.
   This may produce several subsequent PRs without another upstream version bump.

The pinned version defines what we compare against; it does not claim exhaustive
parity. A successful upgrade must leave a valid reference and an accountable
assessment, not an empty parity backlog.

## 1. Upgrade the pinned reference

Use `test/conformance/upstream.yaml` for the starting versions and a fixed, coherent,
mature target for the destination. Update all consumer pins, the lockfile, generated
expectations and reviewed evidence together. Keep the starting references available
for explaining changed behavior. See the [tooling reference](../../../test/conformance/UPGRADING.md)
for selection, application and validation commands.

Run the tests and tooling against the target. Explain snapshot changes and resolve
required-check failures. If the new reference introduces an incompatibility that
prevents a supported integration from working, correct it here or wait to upgrade;
registering a follow-up is not a substitute. This applies even when existing tests
miss the incompatibility. Do not weaken comparisons to make the upgrade green.

These are upgrade blockers, not a requirement to implement every upstream feature
before moving the pins. Other assessed differences can be registered for later work.

## 2. Assess the current Go implementation against the target

Perform a comprehensive comparison across the declared supported surfaces: core
orchestration, provider contracts/adapters, frontend behavior and Gateway. Include
harness limitations when evaluating the evidence. Use relevant `PARITY.md` entries
to identify current support, deviations and coverage.

Inspect exact upstream implementation and tests alongside the current Go paths.
The release delta and changelogs guide investigation, but do not bound it: look for
older gaps too. Trace requests, stream state, continuation, errors and interactions.
Do not infer matching behavior from unchanged files or passing fixtures alone.
Identify unsupported upstream product families separately, and state any area the
assessment could not resolve rather than claiming it was covered.

| Finding | Decision | Evidence needed |
| --- | --- | --- |
| Target breaks required checks or a supported integration | Resolve in the pinned-version upgrade or block it | Reproduction, source/test contract and regression proof |
| Go behavior differs without blocking the reference upgrade | Register a parity correction | Exact difference, impact and proposed acceptance tests |
| Go already matches | Retain it; register missing coverage if needed | Existing proof or a focused comparison |
| New capability within a supported area | Recommend a parity work package or explicit exclusion | Semantics, Go design implications and dependencies |
| Different mechanism, same observable behavior | Retain the Go adaptation | Behavioral and wire equivalence |
| Intentional observable difference or unsupported family | Confirm and document the disposition | Rationale and precise scope boundary |
| Behavior or impact is inconclusive | Investigate; keep the question visible | Missing source, reproduction or design decision |

Implementation and evidence are separate: a capability may work without sufficient
proof, and a passing scenario may cover only part of it. Give new and existing gaps
the same scrutiny. A known bug cannot disappear behind a coverage label, and an
unresolved upgrade-blocking risk cannot be silently assigned to later work.

## 3. Register the parity differences

Give each durable record one responsibility:

- **GitHub issues own actionable deferred work.** One `upstream-sync` issue holds
  the behavior and impact, exact upstream reference, current Go difference,
  intended outcome, API decisions, dependencies, owner and acceptance evidence.
  Do not restate that content in repository documentation or mirror issue state.
- **`PARITY.md` owns stable coverage facts.** Update it only when a surface's
  coverage classification, confidence source, supported boundary or accepted
  deviation changes. Never add a dated/versioned assessment section, issue catalog
  or upgrade-run ledger. A newly discovered actionable difference does not by
  itself require a `PARITY.md` change once an issue owns it.
- **The upgrade PR owns the run ledger.** Record the target, corrections,
  validation and a compact exact list of created or reused issues. Summarize
  adaptations, exclusions and unresolved blockers there; promote only durable
  boundaries or accepted deviations to repository coverage metadata.

Search by behavior/provider across labeled and unlabeled issues and inspect open
and closed matches before creating anything. Reuse covered open work and flag
ambiguous overlaps rather than automatically duplicating it. Apply **`upstream-sync`
to every new or reused parity issue**, adding it when missing without replacing
other labels. Follow the [issue registration rules](../../../test/conformance/UPGRADING.md#issue-creation-and-duplicate-checks)
for search, issue contents and label selection, then link the issue from the
upgrade PR.

Registering work does not mean accepting a permanent deviation or approving a new
API design. Obtain explicit scope/deviation decisions where needed. Reassess open
packages against a later pinned reference instead of blindly carrying stale claims.

The upgrade PR is complete when the pins/evidence are consistent, required checks
pass, the supported-surface assessment is accounted for and remaining differences
are linked to work or an explicit disposition. A version bump alone is incomplete;
a nonempty, honest parity inventory is not itself a failed upgrade.

## 4. Implement parity work independently

Select a registered package and confirm its contract against the current pinned
reference. Refine the Go design and acceptance tests, then implement the complete
behavior with regression proof. Prefer failing conformance cases with authentic
inputs; otherwise use focused tests and state the remaining boundary coverage gap.
Preserve fixture provenance. When work is completed, close or update its issue;
change the coverage map only if the stable coverage status, evidence, support
boundary or accepted deviation changed.

Work packages are behavioral units, not PRs. Related packages may share a PR; a
package spanning published modules may require producer and consumer changes in
separate PRs. Each PR must independently pass required checks, and the package is
complete only when its full outcome and evidence are delivered. Go consumer pins
change when they need published producer behavior, not automatically because the
upstream reference changed.

Review in both directions: **upstream → Go** for missing semantics and edge cases,
and **Go → package** for scope, design justification and regression proof. Use the
parity-review skill for that review. Report package completion separately from
completion of the earlier pinned-version upgrade.
