## Why

Upstream releases continue during implementation and team absences. The current workflow reselects and mutates the baseline before assessment, while local automation duplicates selection policy and can create unmergeable upgrades. We need a resumable workflow that discovers its own scope and produces independently green PRs.

## What Changes

- Separate the registered baseline, frozen candidate target and approved capability backlog.
- Require source-backed assessment and published-module dependency planning before implementation; distinguish baseline verification from completion of the approved rollout.
- **BREAKING**: replace implicit latest selection in the upgrade command with explicit non-mutating selection and application of a saved target. Preserve maturity, coherence, source provenance and all consumer pins.
- Introduce one operational runbook and align repository guidance and parity skills with it.
- Test immutable selection, deterministic application, stale-source rejection, resume and verification-date semantics without network-dependent unit tests.
- Align local automation with read-only discovery and repo-owned policy; remove autonomous upgrades and shared upstream checkout mutation.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `upstream-parity-governance`: frozen-target lifecycle, assessment and independently mergeable delivery, deterministic target tooling and automation boundaries.

## Impact

Conformance upgrade tooling/tests, mise tasks, contributor/agent instructions, parity skills and a new tooling runbook. Local Herdr prompts/config are separately validated operator configuration, not repository artifacts. No SDK runtime change, provider fixture edit, actual upstream upgrade, dependency-policy bypass, autonomous launch or GitHub mutation is included.
