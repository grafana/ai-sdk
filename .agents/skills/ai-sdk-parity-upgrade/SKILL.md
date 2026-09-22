---
name: ai-sdk-parity-upgrade
description: Analyze upstream AI SDK changes, decide their impact on the Go port, plan independently mergeable updates, and review the resulting parity. Use for routine upgrades or catch-up after a longer gap.
---

# AI SDK Parity Upgrade

Turn an upstream version delta into justified implementation decisions and
regression evidence. A version bump or green replay alone does not establish parity.

## 1. Establish the comparison

Use `test/conformance/upstream.yaml` for the registered versions and the supplied
or selected target for the destination. Keep that target fixed during the work.
Consult relevant `PARITY.md` entries for existing support, deviations and evidence.

Compare exact package source and tests between those versions, then inspect the
corresponding Go behavior. Use changelogs to locate changes, not as a complete
specification. Follow adjacent execution paths when a change affects requests,
stream state, continuation, errors or frontend behavior.

Start with this context; read other documentation when a finding requires it.
[Tooling and evidence reference](../../../test/conformance/UPGRADING.md) explains
selection, replay, source checks and their limitations.

## 2. Decide what each change means

Classify observable behavior, not commits or changed files. Several upstream
commits may form one capability; one commit may affect several supported surfaces.

| Finding | Decision | Evidence needed |
| --- | --- | --- |
| Supported upstream behavior changed and Go differs | Correct the implementation; determine whether it must accompany the baseline transition | Exact source/tests and a regression exercising the affected behavior |
| Go already matches | Retain the implementation; strengthen missing coverage rather than re-port it | Existing regression or a focused new comparison |
| New capability within a supported area | Evaluate the Go API and end-to-end behavior; recommend inclusion or explicit deferral | Upstream semantics, dependencies, design implications and a testable scope |
| Different language mechanism, same observable behavior | Keep the Go adaptation | Proof that relevant behavior and wire semantics remain equivalent |
| Deliberately different observable behavior | Seek an explicit deviation decision, not an equivalence claim | Rationale, user impact and regression/coverage boundary |
| Change belongs to an unsupported product family | Confirm the scope exclusion | Evidence that it is separate from supported paths |
| Behavior or evidence is inconclusive | Investigate further; leave the decision unresolved | The missing source, reproduction or design decision |

Track implementation and evidence separately: an implemented capability may lack
proof, and a passing fixture may cover only part of a capability. Review source
changes not exercised by existing fixtures. An untested supported-contract bug
still needs a decision; it cannot disappear behind green replay.

Summarize the assessment in one compact matrix:

| Behavior / surface | Upstream reference | Go behavior | Existing / missing proof | Decision and rationale |
| --- | --- | --- | --- | --- |
| Concrete observable change | Exact source/test reference | Matches, missing or differs | What is actually exercised | Implement, retain, adapt, defer, exclude or investigate |

Obtain approval for new API designs, scope exclusions and intentional deviations.
A proposed deferral is not an accepted gap. Record accepted durable differences in
`upstream.yaml` or `PARITY.md` without weakening existing comparisons.

## 3. Define parity work packages and the baseline transition

Group accepted findings into work packages defined by complete behavior, not PRs.
Each package identifies the upstream contract, Go difference, intended outcome,
exclusions, dependencies and acceptance evidence. Include continuation and frontend
behavior where they are part of that contract.

For a baseline upgrade, advancing the coherent upstream package set is the objective;
whether it can precede particular implementation work is an assessment result.
Do not automatically bump first and defer failures, or require every optional
upstream capability before advancing.

| Finding | Relationship to the baseline transition |
| --- | --- |
| Changed behavior breaks an existing supported contract | Required for the transition, even without an existing failing fixture |
| Existing checks fail against the target | Resolve before the transition lands; recording a follow-up does not satisfy the gate |
| New optional capability independent of existing behavior | Candidate follow-up package, with an explicit scope and coverage decision |
| Existing documented gap or intentional deviation | Reassess against the target; retain only with a valid disposition |
| Impact or evidence is inconclusive | Investigate before assigning it to the transition or a follow-up |
| A separately published producer is needed | Establish producer/consumer ordering; this is a delivery dependency, not an automatic upstream version bump |

The baseline-transition package includes the upstream pins, canonical expectations,
verification and behavior corrections needed to make that transition valid.
Follow-up packages use the new registered baseline and remain visible until their
approved outcomes are delivered. Parity improvements within the existing baseline
do not require an upstream version bump merely to show progress.

Only after defining these packages, derive the delivery plan. Related packages may
share a PR; a package spanning published modules may need a producer PR and later
consumer adoption. The package is complete only when its full acceptance contract
is met. Preserve tests at each delivered boundary. Every PR must independently pass
required checks; a later PR cannot repair its merge readiness. Detailed publication
and merge checks belong in the tooling reference.

Implement each agreed package with its regression proof. Prefer a failing
conformance case when authentic inputs cover it; otherwise use focused tests and
state the provider-boundary coverage gap. Preserve fixture provenance.

## 4. Review in both directions

**Upstream → Go:** does every relevant upstream change have a supported decision?
Check missing capabilities, edge cases and interactions, not just replay failures.

**Go → assessment:** does every implementation change serve an assessed need?
Check scope growth, unnecessary abstractions, incorrect adaptations and missing
regressions. Trace snapshot changes back to behavior rather than accepting churn.

Then verify the appropriate evidence: requests, stream/UI output, hooks, errors,
provider contracts and published consumer behavior. A check's name is not proof
of its breadth; inspect what ran and distinguish omissions from successful checks.

Conclude with implemented behavior, verified evidence and unresolved or accepted
gaps. Distinguish **baseline updated** from **approved capabilities completed**:
changing versions does not finish the rollout, and a deferred capability must
remain visible. Use the parity-review skill for focused implementation review.
