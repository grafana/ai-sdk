# Proposal

## Why

Merged-library authorization is implemented and root #490 has passed live
verification. Publication still has no bounded writer or immutable recovery path.

## What Changes

- Integrate frozen candidate authorization with pinned release-please publication.
- Permit recovery only for matching immutable tag/release state; revalidate before writes.
- Serialize candidate writes and retain pending discovery until lifecycle completion.
- Keep the command and release workflow disabled; use mocked GitHub writes only.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `multi-module-release-management`: bounded publication and immutable retries.

## Impact

Release tooling, executable tests and runbook. No tags/releases, protection edits,
Gateway integration, Renovate activation or runtime parity changes in this delivery.
