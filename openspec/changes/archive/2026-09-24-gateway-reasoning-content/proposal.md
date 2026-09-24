# Gateway reasoning content

## Why

Issue #111 requires reasoning text and files across the ProviderWire boundary. Valid reasoning-only paid responses currently fail during adaptation and can trigger caller retries.

## What Changes

- Accept assistant reasoning text and data/URL reasoning files with scoped options.
- Preserve ordered unary content and concurrent reasoning stream blocks with raw family-specific IDs.
- Transport a bounded continuation metadata projection without exposing operational metadata.
- Extend direct-call file validation, independent client decoding, replay witnesses, bounds and privacy tests.

## Impact

Apache provider/client prerequisites remain independent of the AGPL service. This change is stacked on WP15 PR #235, includes no WP16 or WP21 expansion, and does not change general retry classification. Dependency publication remains a separate verification gate.
