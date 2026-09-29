## Why

Daily pinned-version assessments can discover many deferred parity differences. Repeating their behavior, evidence and acceptance criteria in `PARITY.md` as well as GitHub issues makes the coverage map grow on every run and creates two sources of truth that drift.

## What Changes

- Make `upstream-sync` GitHub issues authoritative for actionable deferred parity work.
- Keep `PARITY.md` focused on stable coverage status, confidence sources, supported boundaries and accepted deviations.
- Keep run-specific disposition lists in the upgrade PR rather than adding dated assessment sections or issue catalogs to repository documentation.
- Update the parity skills, tooling reference, contributor guidance, governance contract and local automation prompt consistently.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `upstream-parity-governance`: Separate ownership of actionable work, durable coverage metadata and run-specific upgrade reporting.

## Impact

This changes parity workflow documentation and review policy only. It does not change runtime code, baseline versions, fixtures, issue contents or provider behavior.
