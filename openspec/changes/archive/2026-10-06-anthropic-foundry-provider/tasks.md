## Implementation

- [x] Compare registered Anthropic baseline and pinned native Foundry transport.
- [x] Add explicit shared Foundry endpoint and credential options.
- [x] Add Foundry model construction and provider-owned canonical response identity.
- [x] Preserve canonical capabilities and published-module compatibility.
- [x] Add synthetic transport/protocol regression tests and document boundaries.
- [x] Run focused provider tests, standalone module checks and parity validation.
- [x] Validate and archive this change.

## Validation evidence

Standalone readonly Azure package tests, focused race tests, workspace tests,
scoped Go lint and docs lint pass. Baseline validation, pinned fixture inventory,
provider discriminator checks and OpenSpec validation pass. Full parity execution
reaches an unrelated Gateway source-capture workspace failure; standalone
conformance execution stops at pre-existing readonly module drift. These limits
are reported rather than represented as passing live/provider parity evidence.
