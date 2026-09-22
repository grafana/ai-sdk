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

The initial configuration restricted the parity automation to repository-owned discovery; this was superseded by the daily-upgrade correction below. Its hook no longer selects an ai-only version, resets work or switches the shared upstream checkout. Command/plan/review prompts route parity work to repo skills/runbook. Generic global agent instructions were inspected and needed no project-specific duplication.

- Original local files backed up outside Git under `/home/nara/.local/share/ai-sdk-artifacts/parity-workflow-local-oAKhy2`.
- Herdr configuration validation passed.
- The initial dry-run lacked `GITHUB_TOKEN`; a second dry-run with explicit placeholder credentials passed. This validates configuration/launch planning only, not authentication or execution. No hooks or agents were launched.
- Both the initial configuration and the corrected daily-upgrade configuration stop until the new workflow exists on the checked-out main; neither uses the old independent selector as fallback.
- The live local config/prompt changes are outside the branch and will need separate operator rollback if desired; the repository diff does not deploy them on other machines.

Raw test and dry-run logs are outside the repository. The initial implementation checks did not perform an upstream upgrade, spec sync/archive, upstream checkout change or automation run. The implementation was subsequently committed and opened as draft PR #205 under separate authorization.

## Decision-led documentation revision

Following review, the upgrade skill now uses a behavioral decision matrix, scope/dependency questions and upstream-to-Go / Go-to-assessment review. The review skill challenges those conclusions, and UPGRADING.md is a command/evidence reference rather than an agent setup workflow. Related repo guidance and change artifacts are aligned; no persistent active OpenSpec change is assumed by the skills.

Docs lint, explicit Markdown lint, relative links, strict OpenSpec validation and whitespace checks passed. This revision changes only Markdown: target tooling, local automation configuration and canonical baseline assets are unchanged. No new assessment-report tool or additional lifecycle modes were introduced.

## Work-package planning revision

The skill now defines behavioral work packages before classifying baseline-transition requirements and follow-ups, then derives PR delivery. Related guidance distinguishes coherent upstream reference bumps from published Go consumer adoption and does not assume packages and PRs map one-to-one. Supported-contract regressions cannot become follow-ups merely because fixtures miss them.

Docs lint, explicit skill/reference Markdown lint, links, strict OpenSpec validation and whitespace checks passed. Only Markdown changed; no runtime/tooling or baseline assets changed. Commit and push of that revision were explicitly requested.

## Pinned-version upgrade and independent parity matching

The next documentation revision separates one pinned-version upgrade/assessment PR from subsequent implementation of registered parity work packages. Assessment compares current Go against the target across supported surfaces, including older gaps; coverage records summarize findings and link actionable issues. Required-check failures or incompatibilities preventing supported integrations from working cannot be hidden as follow-ups. Remaining tracked work is not a claim of full parity, permanent deviation acceptance or API approval.

Repo docs/skills and change artifacts are aligned. Four local prompts were updated to remove the earlier assessment-before-any-pin-mutation wording and distinguish upgrade assessment from parity-package completion. Backups are outside Git at `/home/nara/.local/share/ai-sdk-artifacts/parity-two-step-prompts-3owDBd`; scheduler configuration and discovery-only permissions are unchanged. No automation or GitHub issue creation ran.

Docs/Markdown lint, relative links, strict OpenSpec validation, whitespace checks and Herdr configuration validation passed. This revision is documentation-only; target tooling, canonical pins and fixtures are unchanged.

## Daily automation scope correction

The operator clarified that the daily task must execute the pinned-version upgrade and comprehensive assessment, register follow-up issues and publish a draft PR, not merely discover releases. The local prompt now explicitly authorizes those actions while excluding nonblocking parity-package implementation, PR merging, competing upgrade work and silent material scope decisions. The configuration label and setup messages now describe an upgrade task. The earlier discovery-only spec/design requirements are superseded.

Local config/prompt backups are at `/home/nara/.local/share/ai-sdk-artifacts/parity-daily-upgrade-VsQD2C`. Configuration validation and a placeholder-credential dry-run passed without executing hooks or launching an agent. Repo docs/Markdown lint, links, strict OpenSpec and whitespace checks passed. The actual upgrade was not run; canonical pins, fixtures and runtime tooling remain unchanged. Repository documentation updates are published separately from the machine-local prompt/config.
