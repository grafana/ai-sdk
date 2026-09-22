---
name: ai-sdk-parity-review
description: Review an ai-sdk diff, feature, bug or package against its upstream parity contract. Find behavioral mismatches, unsupported assumptions, scope drift and gaps in regression evidence.
---

# AI SDK Parity Review

Review the requested scope against the registered upstream baseline, or the fixed
target explicitly selected for an upgrade. If no scope is specified, review the
current diff. Use the [upgrade skill](../ai-sdk-parity-upgrade/SKILL.md) for a broader
assessment and implementation plan; use this skill to challenge the resulting work.

## Establish the contract

Read `test/conformance/upstream.yaml` and the relevant `PARITY.md` entries to identify
versions, supported behavior, existing evidence and accepted deviations. For an
upgrade, distinguish the starting baseline from its selected target and decisions.

Inspect matching upstream implementation and tests alongside the Go paths involved.
Prefer exact local Git references, installed package sources or versioned source
URLs. If the required source is unavailable, state the gap rather than substituting
another version. Additional context should answer a concrete review question, not
be a mandatory reading list.

## Review in both directions

### Upstream → Go

- Are the relevant semantics represented, including defaults, optional values,
  warnings/errors, state transitions and continuation?
- Does a language-specific adaptation preserve observable behavior and wire shape?
- Are changes outside existing fixtures accounted for, rather than inferred correct
  from a passing suite?
- Are missing capabilities, unsupported families and intentional differences
  explicitly distinguished?

### Go → requirement

- Does each implementation change address an assessed need within the scope?
- Are new APIs, abstractions or altered behavior justified by the contract?
- Does the regression test fail for the actual bug and exercise the complete path?
- Are snapshot changes explained by behavior, with intact input provenance?

## Evaluate proof and mergeability

Choose evidence appropriate to the affected layer: provider request snapshots,
core UI/output snapshots, frontend hook scenarios, focused provider tests or
Gateway contract/runtime checks. See the [tooling reference](../../../test/conformance/UPGRADING.md)
for what each check establishes and its limitations.

Inspect actual results, including skips and warning-only reports. Separate missing
implementation from missing evidence. Verify published dependencies when a consumer
uses another Go module; workspace tests alone do not prove adoption. Required checks
must pass for this PR without relying on a later unmerged change.

For a baseline transition, check coherent pins, lockfile, generated expectations,
reviewed attestation and verification evidence together. A newer upstream release
alone is not a defect in an implementation targeting a fixed version.

## Report actionable findings

For each finding, provide the behavior at risk, source/code evidence, classification,
recommended correction and required proof. Classify it as an implementation bug,
upstream behavior change, intentional deviation, coverage gap or unresolved design
question. Same-behavior Go adaptations are not findings unless they explain a concern.

Do not list every matching behavior. For broad scope, use a compact area/evidence/
finding/action matrix. State commands actually run and distinguish baseline
verification from completion of the selected feature scope. Do not accept a gap
by omission or suggest weakening comparisons. Durable coverage decisions belong
in `upstream.yaml` or `PARITY.md`; review-specific observations belong in the report.
