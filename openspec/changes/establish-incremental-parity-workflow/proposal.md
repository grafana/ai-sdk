## Why

Upstream releases continue during implementation and team absences. The current workflow reselects and mutates the baseline before assessment, while local automation duplicates selection policy and can create unmergeable upgrades. We need a resumable workflow that discovers its own scope and produces independently green PRs.

## What Changes

- Separate pinned-version upgrades from parity matching: one green reference-update PR includes validation, a comprehensive current-Go-versus-target assessment and registration of remaining work; later PRs implement parity packages independently.
- Include older gaps as well as the release delta. Summarize support/coverage in existing records and link actionable issues, without conflating tracked work with accepted deviations or approved APIs. Distinguish upstream reference bumps from Go consumer dependency adoption.
- **BREAKING**: replace implicit latest selection in the upgrade command with explicit non-mutating selection and application of a saved target. Preserve maturity, coherence, source provenance and all consumer pins.
- Make parity skills decision-led: classify behavior and evidence, derive implementation scope and review upstream-to-Go completeness and Go-to-requirement justification. Keep command details and evidence limits in a tooling reference, and execution mechanics in local automation guidance.
- Test immutable selection, deterministic application, stale-source rejection, resume and verification-date semantics without network-dependent unit tests.
- Align local automation with read-only discovery and repo-owned policy; remove autonomous upgrades and shared upstream checkout mutation.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `upstream-parity-governance`: frozen-target lifecycle, assessment and independently mergeable delivery, deterministic target tooling and automation boundaries.

## Impact

Conformance upgrade tooling/tests, mise tasks, contributor/agent instructions, parity skills and a new tooling runbook. Local Herdr prompts/config are separately validated operator configuration, not repository artifacts. No SDK runtime change, provider fixture edit, actual upstream upgrade, dependency-policy bypass, autonomous launch or GitHub mutation is included.
