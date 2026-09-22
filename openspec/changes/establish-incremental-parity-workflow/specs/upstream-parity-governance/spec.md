## MODIFIED Requirements

### Requirement: Parity upgrade workflow
The repository SHALL distinguish pinned-version upgrades from parity matching. A pinned-version upgrade SHALL update a fixed coherent reference, run verification, comprehensively assess the current Go implementation against the target and register remaining differences. It SHALL produce an independently green PR containing pins, lockfile, expectations, reviewed evidence and necessary compatibility corrections. Subsequent parity matching SHALL process registered behavioral work packages independently. The pinned reference SHALL NOT be presented as exhaustive feature parity. Selection SHALL default to the newest coherent stable package set on npm latest lines satisfying the repository minimum release age at selection time. New releases SHALL NOT automatically change the selected target; changing it or choosing an intermediate checkpoint SHALL require an explicit decision and reassessment.

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
- **THEN** the workflow SHALL assess current Go behavior against the selected target across declared supported surfaces, including older gaps beyond the release delta
- **AND** it SHALL register remaining parity work separately from completion of the pinned-version upgrade

### Requirement: Parity workflow skills
The repository SHALL provide decision-led skills for pinned-version upgrade assessment and independent parity-work review, supported by a tooling/evidence reference. Assessment SHALL compare exact target implementation/tests with the current Go implementation across declared supported surfaces; changelogs, the release delta and existing fixtures SHALL guide but not bound it. It SHALL distinguish upgrade blockers, nonblocking implementation differences, matching behavior, new capabilities, adaptations, intentional deviations, unsupported families and unresolved questions. Implementation status and evidence coverage SHALL be recorded separately. Remaining work SHALL be defined by upstream contract, Go difference, outcome, dependencies and acceptance evidence without requiring a prewritten plan or persistent active change. Review SHALL check assessment completeness for a pinned-version upgrade and full behavioral acceptance for a parity package. Generic agent setup, scheduling and permissions SHALL remain outside the parity skills.

#### Scenario: Agent performs scoped parity review
- **WHEN** reviewing normal development
- **THEN** the skill SHALL use the registered baseline and report problematic classified findings
- **AND** an explicit upgrade SHALL compare against its saved target as well as the starting contract

#### Scenario: Agent performs parity upgrade
- **WHEN** starting an upgrade without a prior plan
- **THEN** the skill SHALL update and validate a fixed target, assess current Go behavior comprehensively and register remaining parity differences
- **AND** completion of that pinned-version upgrade SHALL be distinct from completion of the registered parity packages

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

### Requirement: Independent parity work packages
Parity work packages SHALL represent behavioral outcomes rather than PRs and SHALL be processed independently from the pinned-version upgrade that registered them. Required-check failures and target-induced incompatibilities that prevent supported integrations from working SHALL be resolved before the upgrade lands or SHALL block it, even when existing fixtures miss them. Other differences, including existing implementation gaps and new capabilities, SHALL receive explicit registered work or adaptation/exclusion dispositions without requiring full parity before updating the reference. Unresolved upgrade-blocking risks SHALL NOT be silently deferred.

#### Scenario: Pinned-version upgrade has a compatibility blocker
- **WHEN** the target fails required checks or introduces an incompatibility that prevents a supported integration from working
- **THEN** the correction and proof SHALL accompany or precede the upgrade, or the upgrade SHALL wait
- **AND** recording a parity work package SHALL NOT replace satisfying that gate

#### Scenario: Nonblocking parity differences remain
- **WHEN** assessment identifies an older implementation gap or new capability that does not block the reference upgrade
- **THEN** the upgrade MAY complete with an explicit registered work package or adaptation/exclusion disposition
- **AND** upgrade completion SHALL NOT claim that remaining behavior has been implemented

#### Scenario: Delivery spans multiple PRs
- **WHEN** a work package needs a separately published producer before consumer adoption
- **THEN** its delivery MAY span independently green PRs
- **AND** the package SHALL remain incomplete until its full behavioral acceptance contract is satisfied

#### Scenario: Dependency bumps serve different purposes
- **WHEN** planning an upstream baseline upgrade or a Go consumer update
- **THEN** upstream references SHALL move as the selected coherent set with canonical expectations and verification
- **AND** Go consumer requirements SHALL change according to required published APIs/behavior, not automatically because the upstream baseline changed

### Requirement: Parity difference registration
The pinned-version upgrade SHALL account for all assessed differences using existing coverage records and linked actionable issues, not a separate tracking system. PARITY.md and baseline gap metadata SHALL summarize current support, evidence and dispositions without duplicating full investigations. Issues SHALL identify parity work packages by durable identifier, exact upstream reference, Go difference and impact, intended outcome, design decisions, dependencies, owner and acceptance tests. Registration SHALL NOT imply approval of a new API or acceptance of a permanent deviation.

#### Scenario: Upgrade assessment records older gaps
- **WHEN** current Go behavior differs from the target even though the relevant upstream code did not change during the selected release range
- **THEN** the difference SHALL be included in the assessment and linked work or disposition

#### Scenario: Parity package is delivered
- **WHEN** the full behavioral outcome and regression proof are implemented
- **THEN** the coverage record and linked issue SHALL reflect completion independently of the earlier pinned-version upgrade

#### Scenario: A later upgrade reassesses outstanding work
- **WHEN** the pinned reference advances again
- **THEN** open parity packages SHALL be checked against that reference rather than retaining stale assumptions without review

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

### Requirement: Scheduled pinned-version upgrade and assessment
The configured daily parity automation SHALL execute the repository's pinned-version upgrade and comprehensive assessment workflow, not only discover releases. Its authorization SHALL include necessary compatibility corrections, registration of actionable follow-up issues, signed commits, branch push and creation of a draft upgrade PR. It SHALL NOT automatically implement nonblocking parity packages, approve unresolved material API/scope decisions, merge a PR, mutate a shared upstream checkout or replace another upgrade's fixed target. Local memory SHALL be advisory rather than source or approval authority.

#### Scenario: A newer target is available
- **WHEN** no existing pinned-version upgrade PR is active and tooling selects a newer eligible coherent set
- **THEN** the automation SHALL update and validate the reference, comprehensively assess current Go behavior, register remaining parity work and publish a draft PR when its completion conditions are met

#### Scenario: Related upgrade work already exists
- **WHEN** an open PR already advances the pinned reference, even to an older target
- **THEN** the automation SHALL report it and stop rather than create competing work or replace its target

#### Scenario: Nonblocking parity work is registered
- **WHEN** the assessment identifies a remaining actionable difference
- **THEN** the automation SHALL reuse a matching issue or create a work-package issue and link the relevant coverage record
- **AND** registration SHALL NOT authorize implementation of that package or silently accept a permanent deviation

#### Scenario: No version update is needed
- **WHEN** the selected package set matches the registered baseline
- **THEN** the automation SHALL report no upgrade needed without creating an upgrade PR

#### Scenario: Workflow or upgrade is blocked
- **WHEN** tooling is unavailable or completion requires an unavailable published dependency or unresolved material decision
- **THEN** the automation SHALL preserve and report the blocker without weakening checks or claiming success
