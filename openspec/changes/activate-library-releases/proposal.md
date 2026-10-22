# Proposal

## Why

Library verification and immutable publication now exist. Finish the library rollout
in one activation change, rather than another sequence of preparation slices.
Gateway application delivery is separate and does not gate library activation.

## What Changes

Connect library publication and pinned release PR preparation to the canonical
serialized main-push workflow, gated by an explicit repository mode. Root bootstrap
selects only merged #490; library mode supports configured libraries and excludes
Gateway. Verify protections before writes, wait for current tooling source CI, use
separate read and App write credentials, and enable independent first-party tagged
Renovate updates. Document exact settings and live acceptance steps.

## Capabilities

### Modified Capabilities

- `multi-module-release-management`: staged library activation independent of Gateway.

## Impact

Release tooling/workflow, Renovate and runbook only. Live settings, tags and releases
are not changed during implementation. #245 and #468 remain open for live proof.
