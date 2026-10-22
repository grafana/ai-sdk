# Proposal

## Why

Library PR readiness, read-only release previews and the controlled App PR exercise are implemented, but none authorizes the exact merged revision for later publication. A green PR check or an unrestricted release-please rescan cannot prove that a particular pending library release is still the candidate that was validated.

## What Changes

- Add read-only discovery and authorization of frozen, merged library candidates using pinned release-please 17.6.0. Emit per-candidate decisions and evidence, never tags or releases.
- Bind immutable App/repository identity, release metadata, generated files, PR head, validated base/tree and actual squash merge revision. Reject stale or bypassed merge candidates and ambiguous history.
- Require authentic source CI at the exact merged revision, retained PR readiness evidence for the equivalent tested tree, and fresh selected-library validation in an isolated checkout.
- Preserve independent decisions: ready root may be authorized while unrelated downstream providers remain blocked. Reject Gateway selection in this slice.
- Reassess archived tasks 2.2–2.3 against the merged implementation; document remaining publication, recovery, bootstrap, protection and activation work separately.

## Capabilities

### New Capabilities

None. Release authorization belongs to the existing release-management capability.

### Modified Capabilities

- `multi-module-release-management`: frozen merged-library candidate authorization, immutable evidence binding, independent decisions and a read-only authorization boundary.

## Impact

Expected implementation touches locked JavaScript release tooling, Python readiness helpers and tests, the readiness workflow's evidence output, and release tooling documentation. No Go runtime behavior, version/changelog edits, publication calls, repository protection changes, live exercise enablement, Renovate changes or Gateway integration are included. Existing release workflow guards remain intact. Refs #245 and #468; neither issue is closed by this slice.
