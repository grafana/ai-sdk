# Proposal

## Why

Bedrock derives portable reasoning before applying explicit controls, so callers receive compatibility warnings for settings that never reach Converse. Issue #454 tracks the mismatch against the registered `@ai-sdk/amazon-bedrock` 5.0.118 baseline.

## What Changes

- Short-circuit explicit disabled reasoning before deriving effort or budget.
- Resolve explicit effort or budget first, deriving only missing values and their diagnostics.
- Own a copy of reasoning configuration for request preparation, including provider-default calls.
- Pin request shapes, warnings, namespace precedence, explicit zero budgets and repeated/concurrent reuse in focused tests.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `bedrock-provider`: explicit reasoning precedence, diagnostic accuracy and caller ownership for Converse.

## Impact

The existing Bedrock request resolver and its tests change without a public API, dependency or model capability change. Synthetic transport tests prove serialization and diagnostics, not live AWS acceptance. Nova high-effort token omission and the Sonnet 5.5 between-tools extension remain in place.

## Non-goals

No upstream upgrade, Mantle reasoning change, model routing redesign, new reasoning fields, fixture recording or provider deployment.
