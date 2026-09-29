## Context

The workflow currently asks agents to account for every assessed difference, link issues from coverage, and keep concise proof with the upgrade. Although it warns against duplication, those combined instructions make a per-upgrade section in `PARITY.md` look like the safest completion evidence. Repeated runs would turn the stable coverage map into an issue ledger.

## Goals / Non-Goals

**Goals:**

- Give each kind of parity information one authoritative durable home.
- Preserve complete assessment accountability without growing repository documentation on every run.
- Make review reject duplicated issue detail and dated assessment sections.
- Keep stable coverage evidence and accepted scope boundaries discoverable in the repository.

**Non-Goals:**

- Remove existing evidence or accepted deviations from `PARITY.md`.
- Change issue creation, labeling or duplicate-search requirements.
- Change baseline tooling, runtime behavior or the scope of comprehensive assessment.
- Rewrite PR #230 or its issues.

## Decisions

- `PARITY.md` owns broad coverage status, confidence sources, supported boundaries and accepted deviations. It changes only when one of those durable facts changes.
- An open or closed `upstream-sync` issue owns actionable deferred-work detail: behavior, evidence, impact, scope, dependencies and acceptance criteria. Repository docs do not mirror that content or issue state.
- The upgrade PR owns the run-specific ledger: target, fixes, validation, and compact links to created or reused issues. Matching behavior need not be enumerated; adaptations and exclusions are summarized there unless they establish a durable repository boundary.
- Full research remains external under the existing artifact policy. This does not make the PR or issues optional; it separates concise durable records from investigation material.
- The existing `PARITY.md` on this branch remains evidence-bearing and has no issue ledger or dated assessment section. Add an ownership section rather than rewriting historical Gateway evidence without an independent reason.

## Risks / Trade-offs

- Reviewers cannot find every open parity item by reading `PARITY.md` alone. → Use the `upstream-sync` issue label as the canonical backlog and require the upgrade PR to link the run's exact issue set.
- A broad coverage row may remain `mixed` without naming every defect. → The row states the evidence boundary; issues state actionable defects.
- Existing documentation may already duplicate issue details. → Apply the rule prospectively and remove obvious run-specific sections when touched; avoid a risky unrelated rewrite.
