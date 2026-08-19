## Why

ProviderWire V4 H1 currently proves the contract with several duplicated, manually expanded fixtures and separate request/response scenario definitions. Consolidating mechanical variants and reusing independent conformance oracles will make the evidence easier to review and refresh without expanding H1 into a production runtime.

## What Changes

- Replace duplicated response projections and expanded invalid documents with curated semantic seeds plus deterministic test-only derivation or mutation recipes.
- Add a test-only ProviderWire transport seam that renders selected provider-independent conformance inputs, feeds them through the pinned Gateway client, and compares the result with existing UI expectations.
- Reuse the same response seeds across request-capture and response-consumption scenarios.
- Consolidate artifact ownership and derivation provenance in the interop index.
- Add a non-mutating aggregate verification command and an explicit atomic artifact-refresh command.
- Preserve authentic provider fixture inputs, the legacy runtime, and the H1 contract-only production boundary.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-providerwire-v4`: Refine reproducible evidence, fixture ownership, conformance reuse, and verification/update workflow requirements without changing the HTTP or JSON contract.

## Impact

Affected areas are `gateway/providerwire/v4` test corpora and tests, `test/interop/providerwire-v4` test-only tooling and evidence, ProviderWire mise tasks, parity documentation, and OpenSpec artifacts. No production Go API, runtime behavior, provider recording, npm baseline, or user-facing documentation changes.
