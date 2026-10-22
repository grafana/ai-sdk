# Proposal

## Why

Issue #245 completes the inactive release-please scaffold merged in #21. The
external token-broker configuration is now merged, but library publication,
Gateway plugin releases, and current-candidate authorization still need distinct
validation before automation can be activated.

## What Changes

- Complete selected-library readiness with verified release metadata, canonical
  tagged prerequisites, deterministic owned-source freshness and consumer
  regression tests using pinned dependencies.
- Keep ordinary source PRs on candidate-source CI; unrelated downstream
  standalone failures must not block a ready library.
- Integrate #428's workspace-built Gateway plugin validation and package version
  source; Gateway remains independently versioned and independent of library tags.
- Add linked Gateway release intent through the pinned release-please extension
  mechanism, preserving independent library versions and existing commit intent.
- Validate each immutable release candidate before release-please can publish its
  tag, including retries and delayed runs after newer source merges.
- Prepare narrow Renovate rules for prompt first-party tagged dependency updates.
- Document and test the activation sequence, required-check migration, bootstrap,
  App verification and recovery. Activation requires separate maintainer approval.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `multi-module-release-management`: exact-candidate publication, plugin release
  readiness, linked workspace intent, immutable retries and reviewed activation.
- `module-validation-modes`: distinguish the Gateway plugin application from
  standalone library publication and define selected consumer regression evidence.

## Impact

Changes affect release workflows, `scripts/release_readiness.py`, selected-module
validation, release tooling/tests, release-please configuration, Renovate,
`release/README.md`, contributor guidance and agent instructions.

#21 is already merged, so delivery extends that scaffold in a follow-up PR.
#428 remains open as of 2026-10-20; its plugin contract must be integrated before
activation. Package signing/catalog publication remains #459 and deployment remains
#104. No tags, releases, repository-protection changes, credential writes or
deployments are authorized by this proposal.

This is release governance; the registered Vercel baseline and runtime parity
behavior remain unchanged.
