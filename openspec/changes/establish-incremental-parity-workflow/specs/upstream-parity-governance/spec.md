## MODIFIED Requirements

### Requirement: Parity upgrade workflow
The repository SHALL provide one documented workflow for routine and catch-up upgrades. It SHALL distinguish the registered baseline, a frozen active target and an approved capability backlog. Selection SHALL default to the newest coherent stable package set on npm latest lines satisfying the repository minimum release age at selection time. New releases SHALL NOT automatically change an active target. A target change or intermediate checkpoint SHALL require an explicit decision and incremental reassessment. Each implementation PR SHALL be independently mergeable with all enforced checks passing; implementation, pins, generated expectations, lockfile and attestation needed by a baseline transition SHALL land together.

#### Scenario: Upstream baseline is upgraded
- **WHEN** an assessed and approved baseline transition is implemented
- **THEN** all four parity consumers, manifest, lockfile, expectations and reviewed evidence SHALL identify its frozen target
- **AND** the transition SHALL pass all required checks without depending on a later unmerged PR

#### Scenario: Upgrade reveals divergence
- **WHEN** source review, regenerated snapshots or tests reveal a mismatch
- **THEN** it SHALL be classified with implementation status, evidence and disposition before completion
- **AND** unresolved scope decisions SHALL NOT be treated as accepted gaps

#### Scenario: New releases arrive during implementation
- **WHEN** newer upstream versions become eligible
- **THEN** discovery MAY report them for a later cycle
- **AND** application and review SHALL continue against the active target unless its change is explicitly approved

#### Scenario: Team resumes after an absence
- **WHEN** the upstream delta accumulated over an extended absence
- **THEN** the workflow SHALL assess the complete selected range and define behavioral work packages before grouping delivery into independently mergeable PRs

### Requirement: Parity workflow skills
The repository SHALL provide decision-led skills for parity assessment and review, supported by a tooling/evidence reference. Upgrade assessment SHALL compare exact upstream implementation/tests with Go behavior and classify each relevant finding using a decision matrix. It SHALL distinguish changed supported behavior, already-matching implementation, new capabilities, behavior-preserving adaptations, intentional deviations, unsupported families and unresolved questions. Implementation status and evidence coverage SHALL be recorded separately. The assessment SHALL define parity work packages by upstream contract, Go difference, outcome, exclusions, dependencies and acceptance evidence before determining PR boundaries. It SHALL classify their relationship to the baseline transition without requiring a prewritten upgrade plan or persistent active change. Review SHALL check upstream-to-Go completeness and Go-to-assessment scope and justification. Generic agent setup, scheduling and permissions SHALL remain outside the parity skills.

#### Scenario: Agent performs scoped parity review
- **WHEN** reviewing normal development
- **THEN** the skill SHALL use the registered baseline and report problematic classified findings
- **AND** an explicit upgrade SHALL compare against its saved target as well as the starting contract

#### Scenario: Agent performs parity upgrade
- **WHEN** starting an upgrade without a prior plan
- **THEN** the skill SHALL assess scope and dependency order before mutating canonical pins
- **AND** baseline promotion SHALL be distinct from completion of the approved capability rollout

#### Scenario: Agent reviews a broad scope
- **WHEN** review covers a provider or directory rather than only a diff
- **THEN** it SHALL identify missing behavior and missing evidence separately, including changes not exercised by current fixtures

#### Scenario: Implementation is reviewed against the assessment
- **WHEN** the proposed implementation is reviewed
- **THEN** every relevant upstream behavior SHALL have a justified disposition
- **AND** every Go implementation change SHALL serve an assessed requirement with appropriate regression proof

#### Scenario: Evidence has limited breadth
- **WHEN** a check covers fixture provenance, selected discriminators or only specific scenarios
- **THEN** the reference SHALL state that limitation
- **AND** passing the check SHALL NOT be presented as exhaustive behavioral parity

## ADDED Requirements

### Requirement: Work-package planning precedes delivery grouping
Parity work packages SHALL represent behavioral outcomes rather than PRs. Assessment SHALL identify transition requirements, prerequisites and approved follow-ups before grouping changes for delivery. Known supported-contract regressions and existing failures against the target SHALL be resolved before the transition lands; absence of a failing fixture SHALL NOT silently justify deferral. New optional capabilities SHALL have explicit inclusion or follow-up decisions, and existing gaps/deviations SHALL be reassessed against the target. Unknown impact SHALL remain unresolved until investigated.

#### Scenario: Baseline transition has required behavior corrections
- **WHEN** the target changes an existing supported contract incompatibly
- **THEN** its full correction and proof SHALL accompany or precede the baseline transition
- **AND** it SHALL NOT be replaced by a follow-up entry merely to advance dependency versions

#### Scenario: Optional capability can follow the transition
- **WHEN** a new capability is independent of existing supported behavior
- **THEN** assessment SHALL recommend inclusion or an explicit follow-up scope and coverage decision
- **AND** baseline verification SHALL NOT claim that the follow-up capability is implemented

#### Scenario: Delivery spans multiple PRs
- **WHEN** a work package needs a separately published producer before consumer adoption
- **THEN** its delivery MAY span independently green PRs
- **AND** the package SHALL remain incomplete until its full behavioral acceptance contract is satisfied

#### Scenario: Dependency bumps serve different purposes
- **WHEN** planning an upstream baseline upgrade or a Go consumer update
- **THEN** upstream references SHALL move as the selected coherent set with canonical expectations and verification
- **AND** Go consumer requirements SHALL change according to required published APIs/behavior, not automatically because the upstream baseline changed

### Requirement: Frozen target selection and application
Selection SHALL write a new versioned target record without modifying the registered baseline or parity consumers. The record SHALL identify its source baseline, selection time, maturity policy, exact package versions, publication times and upstream source commits. Application SHALL require an explicit target, validate exact-version evidence and every affected manifest before writing, and SHALL NOT reselect latest. Selection SHALL refuse to overwrite an existing record.

#### Scenario: Repeat selection cannot overwrite an active target
- **WHEN** selection is requested with an existing output path
- **THEN** it SHALL fail without changing that record or tracked baseline files

#### Scenario: Saved target is applied after newer releases
- **WHEN** exact target evidence remains available and valid
- **THEN** application SHALL use the saved versions without querying latest

#### Scenario: Invalid or stale target
- **WHEN** the baseline identity, age policy, package inventory, source commit, maturity or dependency coherence is incompatible with the record
- **THEN** application SHALL fail before changing any baseline or consumer manifest

#### Scenario: Resume the same transition
- **WHEN** the current manifest and consumer versions already match the saved target
- **THEN** application SHALL be idempotent and preserve recorded verification and gap metadata
- **AND** an unrelated or mixed baseline SHALL require reassessment instead

### Requirement: Honest baseline verification date
Applying an unverified target SHALL clear the previous verification date and SHALL NOT set a new date. Baseline validation SHALL reject an absent or invalid verification date. A maintainer SHALL record the date only after reviewing successful verification evidence, then rerun the complete gate on the merge candidate.

#### Scenario: Selection is not certification
- **WHEN** selection or application succeeds
- **THEN** it SHALL NOT claim that target parity has passed

#### Scenario: Incomplete transition cannot pass metadata validation
- **WHEN** a new target has no valid verification date
- **THEN** baseline validation SHALL report incomplete verification

### Requirement: Publication-aware independent mergeability
Upgrade plans SHALL account for published Go module dependencies before implementation. Producer APIs/behavior required by a separately resolved consumer SHALL be merged and publicly resolvable before the consumer adopts them. Each delivered PR SHALL run the existing module-resolution gate and required parity/interop checks on its own proposed merge state; the enclosing parity work package SHALL additionally satisfy its complete behavioral acceptance contract.

#### Scenario: Workspace masks an unpublished dependency
- **WHEN** a consumer passes only with a workspace copy of a producer change
- **THEN** planning SHALL allocate a green producer prerequisite and subsequent consumer adoption
- **AND** it SHALL NOT weaken checks, add production replacements or remove regression tests to permit a red merge

### Requirement: Read-only upstream discovery automation
Scheduled discovery SHALL use repository-owned policy and SHALL NOT implement upgrades, replace active targets, mutate a shared upstream checkout, approve gaps, commit, push or create/update PRs. Missing workflow tooling or an active transition SHALL be reported without falling back to an independent selector or autonomous upgrade. Local memory SHALL be advisory rather than target authority.

#### Scenario: Automation sees a newer release during active work
- **WHEN** discovery finds newer eligible versions
- **THEN** it SHALL report them separately without altering the active cycle

#### Scenario: Workflow tooling is not deployed
- **WHEN** the checked-out repository lacks the required workflow entry points
- **THEN** discovery SHALL stop safely and report the deployment prerequisite
