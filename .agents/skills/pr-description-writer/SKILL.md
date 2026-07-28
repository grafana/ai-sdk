---
name: pr-description-writer
description: Write or rewrite pull request descriptions from a branch diff, commit range, existing PR, issue, upstream reference, specification, bug report, or review context. Use when an agent needs to create, update, or improve a PR body so it explains concrete code and behavior changes, why they were made, validation performed, and remaining risk.
---

# PR Description Writer

Use this skill to produce PR descriptions that help reviewers understand what
changed and why. The description must explain concrete behavior and code
changes, not just list touched files, dependencies, generated artifacts, or
snapshot churn.

## Workflow

1. Inspect the available context:
   - current diff, branch range, or PR diff
   - commit list and commit messages
   - existing PR body, issue, bug report, spec, design doc, or upstream source
   - validation commands and residual risks
2. Group changes by reviewer-relevant behavior or subsystem. Prefer categories
   such as API behavior, provider conversion, stream/wire format, UI behavior,
   persistence, docs/examples, tests/fixtures, or tooling.
3. For each group, explain:
   - what changed in code or behavior
   - why the change was needed
   - what evidence or reference supports it
4. Reference upstream/source context when it materially explains the change:
   upstream implementation, tests, release notes, specs, issues, review
   comments, or conformance fixtures. Mention references next to the relevant
   change, not only in a generic footer.
5. Separate generated or mechanical updates from semantic changes. Generated
   snapshots, lockfiles, schema output, or formatter churn should support the
   behavioral explanation, not replace it.
6. Include validation commands actually run. Do not imply checks passed if they
   did not run.
7. Include residual risk, accepted deviations, skipped tests, or follow-up work
   when relevant.
8. Keep the result concise enough for reviewers to scan. Prefer precise bullets
   over broad prose.

## Structure

Use this structure by default and omit sections that do not apply:

```markdown
## Summary

- One to four bullets describing the outcome in reviewer-facing terms.
- Include the main code/behavior changes, not only version or dependency bumps.

## Code Changes

- Area: concrete change, reason, and reference when useful.
- Area: concrete change, reason, and reference when useful.

## Tests And Fixtures

- What tests, fixtures, snapshots, or generated artifacts changed and what they
  prove.

## Validation

- `command`
- `command`

## Residual Risk

- Accepted gaps, intentional deviations, skipped fixtures/tests, blockers, or
  follow-ups.
```

For small PRs, merge `Code Changes` and `Tests And Fixtures` into `Summary`.
For release, parity, migration, or generated-heavy PRs, keep the sections
separate so reviewers can distinguish behavior from generated output.

## Quality Bar

- Avoid vague bullets like "updates provider code" or "regenerates snapshots"
  unless followed by the behavior that changed.
- Avoid file-by-file summaries unless each file maps cleanly to a reviewer
  concern.
- Do not describe unrelated opportunities discovered during review as PR scope.
- Do not overstate certainty. Use "documents residual risk" or "leaves follow-up"
  when behavior remains intentionally incomplete.
- Preserve conventional PR titles and repository-specific PR norms.
