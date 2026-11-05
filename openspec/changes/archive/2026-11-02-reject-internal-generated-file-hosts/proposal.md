## Why

Generated-file downloads accept `.internal` hostnames when DNS returns only
public addresses, while the registered `@ai-sdk/provider-utils@5.0.68` rejects
them. Issue #608 tracks this defense-in-depth parity difference; private DNS
answers are already rejected, and this is not a known vulnerability claim.

## What Changes

- Reject `.internal` hostname suffixes in initial generated-file URLs and redirects.
- Preserve existing case folding, trailing-dot normalization and address checks.
- Prove rejection before DNS lookup or connection to the rejected destination.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `provider-v4-content-model`: specify generated-file `.internal` hostname rejection.

## Impact

Touches the root generated-file URL validator and focused downloader tests.
No public API, provider wire or UI format changes. Prompt URL downloading (#225),
upstream baseline upgrades and unrelated address validation are out of scope.
