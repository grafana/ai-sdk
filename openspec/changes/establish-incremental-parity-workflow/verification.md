# Verification

Implementation base: `origin/main` at `35fb5140ca521873acc3339bf22800289537d62a`.
This change modifies upgrade tooling/policy, not upstream runtime behavior.

## Repository checks

- Target tests were red before select/apply exports existed; date tests were red before verification-date validation existed.
- Conformance tooling suite: 77 tests passed, including the existing selector and new frozen-target/date tests. Typecheck and registered baseline validation passed.
- `mise run parity-check` passed; provider-shape inspected and matched the five actual discriminator families, not a missing-source skip.
- `mise run verify-module-resolution` passed with its fresh public cache and workspace disabled.
- Docs lint, explicit Markdown lint for runbook/skills/conformance README, workflow relative links, strict OpenSpec validation and diff whitespace checks passed.
- `gofmt -l .` is empty. `mise run fmt-check` reports the existing uncommitted implementation diff because that task checks all Git changes after formatting; it is not a clean-tree CI result. It produced no Go changes. Full CI still runs on the eventual committed merge candidate.
- Canonical upstream versions, package dependency pins, lockfile, provider fixture inputs/expectations and Gateway attestation remain unchanged.

## Workflow walkthrough and limits

Synthetic tooling fixtures start from baseline metadata without an upgrade plan, select a mature coherent target, preserve it on disk, apply it after a month without latest selection, update four consumers, clear the old verification date, resume idempotently and reject stale/mixed inputs without mutation. Missing source evidence and a malformed later consumer also fail before writes. These are tooling fixtures, not provider conformance inputs.

This proves the mechanical target lifecycle, not semantic completeness of an AI-generated assessment. The first approved real cycle must independently inventory exact source/tests, obtain scope decisions and produce green packages; the old upgrade plan can then serve as a completeness cross-check. No real upstream upgrade or online target selection was executed here.

## Local operator configuration

The parity automation now invokes repository-owned discovery only. Its hook no longer selects an ai-only version, resets work or switches the shared upstream checkout. Command/plan/review prompts route parity work to repo skills/runbook. Generic global agent instructions were inspected and needed no project-specific duplication.

- Original local files backed up outside Git under `/home/nara/.local/share/ai-sdk-artifacts/parity-workflow-local-oAKhy2`.
- Herdr configuration validation passed.
- The initial dry-run lacked `GITHUB_TOKEN`; a second dry-run with explicit placeholder credentials passed. This validates configuration/launch planning only, not authentication or execution. No hooks or agents were launched.
- Scheduled discovery deliberately stops until the new workflow exists on its checked-out main. The old autonomous upgrade is not used as fallback.
- The live local config/prompt changes are outside the branch and will need separate operator rollback if desired; the repository diff does not deploy them on other machines.

Raw test and dry-run logs are outside the repository. No commit, push, PR mutation, spec sync/archive, upstream checkout change or automation run was performed.
