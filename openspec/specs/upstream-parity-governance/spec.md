# upstream-parity-governance Specification

## Purpose

Define repository standards for declaring, checking, upgrading, and reviewing
parity with the upstream Vercel AI SDK baseline.
## Requirements

### Requirement: Upstream parity baseline manifest

The repository SHALL check in a manifest recording the verified AI SDK parity target: upstream repository URL, coherent TypeScript versions for all parity consumers/reference tooling, verification status/commands and known intentional deviations or accepted gaps. Direct consumers and transitive authoritative reference packages SHALL be registered to enforce coherence.

#### Scenario: Contributor finds the verified upstream target
- **WHEN** a contributor needs to know which upstream AI SDK version this repository is verified against
- **THEN** the upstream parity baseline manifest identifies the upstream repository and package versions used for verification

#### Scenario: Baseline records known gaps
- **WHEN** the repository intentionally diverges from upstream behavior or lacks coverage for an upstream surface
- **THEN** the upstream parity baseline manifest records the deviation or gap with enough context for reviewers to classify it as intentional

#### Scenario: Package source commits differ
- **WHEN** a selected coherent package set contains packages published from different upstream commits
- **THEN** the retained target record identifies each exact source commit
- **AND** semantic review uses the matching package source and tests

### Requirement: Transition target source provenance

Pinned-version transitions SHALL retain their selected target record with the change, including exact per-package source commits rather than assuming every package shares the manifest upstream commit.

#### Scenario: Transition target source provenance
- **WHEN** a coherent target includes packages published from different commits
- **THEN** the retained record SHALL identify exact per-package commits for matching implementation/test review

### Requirement: Parity coverage map

The repository SHALL maintain a stable coverage map classifying surfaces by status, layer, confidence source, durable support boundary and accepted deviation. It SHALL cover at minimum core orchestration/UI chunks, provider contract, provider implementation, frontend interop and conformance harness. Status SHALL be automated, manual, documented deviation, mixed coverage or gap.

#### Scenario: Reviewer evaluates compatibility coverage
- **WHEN** a reviewer inspects a change touching a parity-sensitive surface
- **THEN** the coverage map identifies the affected layer and whether that surface is protected by automated conformance, manual review, documented deviation, mixed coverage, or an explicit gap

#### Scenario: New parity surface is added
- **WHEN** implementation adds a new upstream compatibility surface
- **THEN** the coverage map is updated to classify how the new surface is verified

#### Scenario: Provider implementation work is reviewed
- **WHEN** implementation changes provider request conversion, provider response parsing, provider-defined tools, or provider options
- **THEN** the coverage map identifies the provider implementation capability and whether request snapshots, stream snapshots, manual review, or a durable boundary provide confidence

#### Scenario: Core ai-sdk work is reviewed
- **WHEN** implementation changes orchestration, stream part conversion, UI chunk output, tools, or structured output
- **THEN** the coverage map identifies the core capability and whether UI chunk snapshots, structured output snapshots, manual review, or a durable boundary provide confidence

#### Scenario: Actionable deferred work is registered
- **WHEN** a parity difference has a new or reused `upstream-sync` issue
- **THEN** the issue SHALL remain authoritative for the behavior, evidence, impact, scope, dependencies and acceptance criteria
- **AND** the coverage map SHALL change only if the surface's coverage classification, confidence source, supported boundary or accepted deviation changed

### Requirement: Coverage map is not an upgrade ledger

The coverage map SHALL NOT serve as a version-upgrade ledger or actionable-work queue. It SHALL NOT contain dated target assessments, catalogs of `upstream-sync` issues or copies of issue behavior, impact, scope, dependencies, acceptance criteria or state.

#### Scenario: Coverage map is not an upgrade ledger
- **WHEN** an upgrade registers deferred actionable parity work
- **THEN** the map SHALL retain stable coverage information without copying issue details, state or dated target assessments

### Requirement: Parity check command
The repository SHALL provide a standard parity check command that validates the upstream parity baseline and runs the relevant automated compatibility checks. The command SHALL validate manifest consistency with every parity TypeScript consumer dependency pin, typecheck conformance tooling, and run conformance tests or a documented stable conformance subset.

#### Scenario: Developer checks current parity
- **WHEN** a developer runs the parity check command
- **THEN** the command validates baseline metadata consistency and executes the configured automated parity checks

#### Scenario: Baseline package drift
- **WHEN** the manifest declares a TypeScript package version that differs from a parity consumer dependency pin
- **THEN** the parity check command fails with a diagnostic identifying the consumer and mismatched package

#### Scenario: Upgrade candidate package set is incoherent
- **WHEN** a parity upgrade candidate declares conflicting exact stable dependency versions for another tracked AI SDK package
- **THEN** the upgrade workflow rejects that candidate instead of selecting incompatible parity reference stacks

### Requirement: Parity upgrade workflow

The repository SHALL distinguish pinned-version upgrades from independent parity matching. Upgrades SHALL update a fixed coherent reference, verify it, comprehensively assess current Go against target and register remaining differences. Each SHALL produce an independently green PR with pins, lockfile, expectations, reviewed evidence and necessary compatibility corrections. Subsequent matching SHALL process registered behavioral packages independently; pins SHALL NOT imply exhaustive parity.

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

### Requirement: Mature coherent upgrade selection

Selection SHALL default to newest coherent stable packages on npm latest lines satisfying repository minimum release age at selection. New releases SHALL NOT automatically change selected target; changing it or choosing an intermediate checkpoint SHALL require explicit decision and reassessment.

#### Scenario: Mature coherent upgrade selection
- **WHEN** new eligible packages appear during a frozen upgrade
- **THEN** selection SHALL remain unchanged absent explicit decision and reassessment

### Requirement: Agent parity guidance
Repository agent guidance SHALL define parity-sensitive work and the required review posture for that work. The guidance SHALL require upstream source or test comparison during planning for parity-sensitive changes, conformance fixture consideration for wire or provider-boundary behavior, and explicit classification of differences as parity-preserving Go adaptation, intentional deviation, or bug.

#### Scenario: Agent works on parity-sensitive code
- **WHEN** an agent changes stream parts, UI chunks, provider messages, provider request conversion, tool orchestration, output behavior, provider options, or frontend interop behavior
- **THEN** the agent guidance requires upstream comparison and conformance consideration before implementation is considered complete

#### Scenario: No upstream equivalent exists
- **WHEN** a change has no direct upstream TypeScript equivalent
- **THEN** the agent guidance requires the absence of upstream equivalent to be stated and the compatibility impact to be classified

#### Scenario: Reported bug can be captured by conformance
- **WHEN** a reported bug can be represented by recorded provider chunks, provider request snapshots, or structured output snapshots
- **THEN** the agent guidance recommends adding or updating the conformance fixture first, observing the Go replay failure, and then implementing the fix

#### Scenario: New parity-sensitive feature is implemented
- **WHEN** a new feature affects upstream-visible provider or UI behavior
- **THEN** the agent guidance recommends recording or importing upstream behavior alongside the implementation so conformance acts as the regression contract

### Requirement: Parity workflow skills

The repository SHALL provide decision-led skills for pinned-version assessment and independent parity-work review with a tooling/evidence reference. Assessment SHALL compare exact target implementation/tests against current Go across declared supported surfaces; changelogs, release delta and fixtures SHALL guide but not bound assessment. Generic agent setup, scheduling and permissions SHALL remain outside these skills.

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

### Requirement: Parity assessment classification and evidence

Assessment SHALL distinguish upgrade blockers, nonblocking implementation differences, matching behavior, new capabilities, adaptations, intentional deviations, unsupported families and unresolved questions. Implementation status and evidence coverage SHALL be recorded separately.

#### Scenario: Parity assessment classification and evidence
- **WHEN** assessment finds implemented behavior lacking regression evidence
- **THEN** implementation status and missing evidence SHALL be recorded separately within the appropriate classified disposition

### Requirement: Parity skill work outcomes and review gates

Remaining work SHALL be defined by upstream contract, Go difference, outcome, dependencies and acceptance evidence without requiring a prewritten plan or persistent active change. Review SHALL check assessment completeness for pinned-version upgrades and full behavioral acceptance for parity packages.

#### Scenario: Parity skill work outcomes and review gates
- **WHEN** a parity package is reviewed without a persistent active change
- **THEN** review SHALL check its upstream contract, Go difference, outcome, dependencies and full behavioral acceptance evidence rather than require a prewritten plan

### Requirement: Baseline validation covers all parity TypeScript consumers
The repository SHALL validate that every retained test package consuming `ai` or `@ai-sdk/*` packages uses versions compatible with the registered upstream parity baseline.

#### Scenario: Integration package consumes upstream AI SDK packages
- **WHEN** `test/integration/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

#### Scenario: CLI package consumes upstream AI SDK packages
- **WHEN** `test/cli/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

#### Scenario: Conformance tools consume upstream AI SDK packages
- **WHEN** `test/conformance/tools/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

#### Scenario: Parity upgrade updates every retained consumer
- **WHEN** the registered package set is upgraded
- **THEN** conformance, integration, and CLI package manifests that consume a tracked package are updated together

### Requirement: Provider API-shape drift report
The repository SHALL provide a provider V4 API-shape drift report that compares upstream LanguageModelV4 discriminator values with Go provider constants. By default, the report SHALL resolve the `@ai-sdk/provider` source version declared in the upstream parity baseline. An explicitly supplied source root MAY override that source, but the report SHALL NOT silently fall back to an arbitrary local checkout.

#### Scenario: Upstream adds a stream part type
- **WHEN** upstream LanguageModelV4 declares a stream part discriminator missing from Go provider constants
- **THEN** the drift report identifies the missing value as parity review input

#### Scenario: Registered provider source is selected
- **WHEN** the drift report runs without an explicit source-root override
- **THEN** it reads `@ai-sdk/provider` from the registered baseline and compares against that installed package source

#### Scenario: Registered provider source is unavailable
- **WHEN** the registered provider package source is not installed
- **THEN** the drift report reports that the registered source is unavailable and does not substitute another upstream version silently

### Requirement: Frontend hook interop coverage
The repository SHALL include hook-level frontend interop tests for upstream AI SDK UI consumers at the package versions registered in `test/conformance/upstream.yaml`. A behavior SHALL count as hook-level evidence only when an assertion executes through the corresponding public upstream hook surface; lower-level chunk snapshots, SSE parsing, or UI-message reader tests SHALL NOT substitute for hook state-machine evidence.

#### Scenario: React chat hook consumes Go UI stream
- **WHEN** integration tests run
- **THEN** `useChat`-level tests verify Go SSE consumption through the upstream React hook surface
- **AND** covered tests include successful status ordering, HTTP and stream errors, stop with retained partial output, a multi-step boundary, and approved and denied tool approval responses

#### Scenario: React completion hook consumes Go streams
- **WHEN** integration tests run
- **THEN** `useCompletion`-level tests cover successful consumption, error callback and loading reset, and stop with retained partial completion

#### Scenario: React object hook consumes and validates Go streams
- **WHEN** integration tests run
- **THEN** `useObject`-level tests cover a successful object and a completed schema-invalid object
- **AND** the schema-invalid test asserts the final `onFinish` result

#### Scenario: Lower-level evidence remains distinct
- **WHEN** a parity claim is supported only by chunk snapshots, SSE parsing, or UI-message reader tests
- **THEN** the coverage map identifies that proof as lower-level evidence rather than hook-level state-machine coverage

### Requirement: Frontend parity status reflects covered breadth
Frontend capability statuses in `test/conformance/PARITY.md` SHALL reflect the breadth of behavior directly exercised at the stated layer. A broad capability with automated coverage for only a subset of its state transitions or failures MUST be classified as `mixed`, with the automated subset and remaining gap described in its confidence source or notes.

#### Scenario: Hook surfaces have partial state-machine coverage
- **WHEN** the suite automates the hook scenarios required by this change but does not exhaustively cover each public hook lifecycle and failure path
- **THEN** the broad `useChat`, `useCompletion`, and `useObject` compatibility rows are classified as `mixed`
- **AND** each row describes the behavior that is automated through the hook

#### Scenario: Chunk ordering evidence is broader than hook evidence
- **WHEN** conformance snapshots automate chunk ordering for fixtures but React assertions cover only selected state transitions
- **THEN** the broad chunk ordering and state transitions row is classified as `mixed`
- **AND** its notes distinguish conformance stream ordering from React hook state transitions

### Requirement: Independent parity work packages

Parity work packages SHALL represent behavioral outcomes, not PRs, processed independently of their registering upgrade. Required-check failures and target-induced incompatibilities preventing supported integrations SHALL be corrected before landing or block the upgrade, even if fixtures miss them. Unresolved upgrade-blocking risks SHALL NOT be silently deferred.

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

### Requirement: Nonblocking difference dispositions

Other differences, including existing implementation gaps and new capabilities, SHALL receive explicit registered work or adaptation/exclusion dispositions without requiring full parity before updating the reference.

#### Scenario: Nonblocking difference dispositions
- **WHEN** assessment finds an older nonblocking implementation gap
- **THEN** the upgrade SHALL register work or explicit adaptation/exclusion disposition without claiming full parity

### Requirement: Parity difference registration

Pinned-version upgrades SHALL account for all assessed differences via corrections, `upstream-sync` issues or explicit adaptation/exclusion dispositions. One issue SHALL represent a coherent actionable remaining package, not per commit/assertion, and be the authoritative durable record of behavior, exact upstream reference, current Go difference/impact, intended outcome, design decisions, dependencies and acceptance tests. Registration SHALL NOT approve a new API or accept a permanent deviation.

#### Scenario: Upgrade assessment records older gaps
- **WHEN** current Go behavior differs from the target even though the relevant upstream code did not change during the selected release range
- **THEN** the difference SHALL be included in the assessment and assigned linked work or an explicit disposition
- **AND** actionable detail SHALL live in its `upstream-sync` issue rather than a dated coverage-map section

#### Scenario: Parity package is delivered
- **WHEN** the full behavioral outcome and regression proof are implemented
- **THEN** the issue SHALL reflect completion
- **AND** the coverage map SHALL change only when the delivered work changes its stable classification, confidence source, supported boundary or accepted deviation

#### Scenario: A later upgrade reassesses outstanding work
- **WHEN** the pinned reference advances again
- **THEN** open parity packages SHALL be checked against that reference rather than retaining stale assumptions without review
- **AND** the new upgrade PR SHALL identify the reused issues without copying their contents into repository documentation

#### Scenario: Existing work covers a finding
- **WHEN** an open issue's scope and acceptance criteria cover the assessed finding, even without an upstream-sync label
- **THEN** registration SHALL reuse and link that issue from the upgrade PR rather than create a duplicate
- **AND** it SHALL add the required upstream-sync label if missing while preserving existing metadata

#### Scenario: Closed or partial matches exist
- **WHEN** similar work was closed or only partially covers the new finding
- **THEN** registration SHALL inspect its resolution and scope before deciding
- **AND** it SHALL flag ambiguous overlaps rather than automatically recreate, reopen or rewrite the issue
- **AND** a distinct proven regression SHALL explain and link its relationship to prior work

### Requirement: Parity registration duplicate search

Registration SHALL search provider/capability/behavior across labeled and unlabeled issues and inspect open and closed matches, including scope, acceptance criteria and resolution. Covered open work SHALL be reused; ambiguous overlap or rejected work SHALL NOT automatically produce a duplicate or reopened issue.

#### Scenario: Parity registration duplicate search
- **WHEN** a closed issue partly overlaps an assessed behavior
- **THEN** registration SHALL inspect resolution and scope, flag ambiguity and avoid automatic duplicates or reopening

### Requirement: Upgrade PR owns run-specific registration

The upgrade PR SHALL identify target, included corrections, validation and a compact exact list of created/reused issues. Repository coverage records SHALL NOT mirror actionable issue content/state and SHALL change only for durable coverage, evidence, support-boundary or accepted-deviation changes.

#### Scenario: Upgrade PR owns run-specific registration
- **WHEN** an upgrade reuses outstanding issues without changing durable coverage
- **THEN** the PR SHALL list them exactly and identify target/corrections/validation while coverage records remain free of mirrored issue content or state

### Requirement: Parity issue labels

Every new/reused parity issue SHALL carry `upstream-sync`, verified after creation/reuse; missing labeling SHALL leave registration incomplete. Optional labels SHALL use existing inventory: bug for incorrect existing behavior, enhancement for capabilities/coverage improvements, documentation for docs-only, question for concrete unresolved investigations. Existing labels SHALL be preserved.

#### Scenario: New parity work is registered
- **WHEN** an actionable work package has no covering issue after duplicate checks
- **THEN** its new issue SHALL include upstream-sync and appropriate existing optional labels
- **AND** its title SHALL identify the area/observable behavior and its body SHALL contain evidence, scope, dependencies and acceptance tests

#### Scenario: Required label cannot be applied
- **WHEN** upstream-sync is unavailable or the label operation fails
- **THEN** registration SHALL report the blocker rather than silently publish the work as fully registered

### Requirement: Parity registration metadata and security limits

Registration SHALL NOT invent labels, infer assignees or automatically apply triage, release, automerge, severity or unrelated workflow labels. Security findings SHALL follow repository private reporting policy instead of ordinary public issue registration.

#### Scenario: Parity registration metadata and security limits
- **WHEN** assessment identifies a security finding
- **THEN** it SHALL follow private reporting rather than public issue registration; registration SHALL NOT infer assignees or add invented or unrelated workflow labels

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

Upgrade plans SHALL account for published Go dependencies before implementation. Coordinated producer/consumer changes MAY share an independently green candidate-source PR when required source parity/interop tests exercise them together and internal pins stay real, downloadable, replacement-free and merged in canonical main. Required checks SHALL NOT rely on later unmerged source. The enclosing parity package SHALL still satisfy full behavioral acceptance.

#### Scenario: Workspace masks an unpublished dependency
- **WHEN** a coordinated source PR passes only with candidate workspace copies of producer and consumer changes while existing published pins are older but merged
- **THEN** its required checks SHALL prove integrated candidate behavior and parity/interop independently of later PRs
- **AND** standalone compilation failure SHALL NOT alone block source merge or Gateway container publication after same-revision image validation, but SHALL block an affected independently consumable Go module's publication until resolved

#### Scenario: Parity upgrade has a failing required check
- **WHEN** a pinned-version upgrade fails a required parity, integration or candidate-source check
- **THEN** it SHALL correct that failure in the upgrade PR or wait rather than registering it as deferred nonblocking work

#### Scenario: Gateway publication at a coherent candidate revision
- **WHEN** a Gateway image validates with candidate SDK/provider/middleware source while its declared older published module pins cannot compile standalone
- **THEN** it MAY publish only after source, ancestry, boundary, image and smoke checks for that revision succeed
- **AND** its success SHALL NOT authorize any SDK/provider/middleware module tag without independent standalone verification

### Requirement: Source integration is not Go module release evidence

Source integration SHALL NOT count as independently consumable Go-module release evidence. Before manually publishing a selected SDK/provider/middleware module, maintainers SHALL validate standalone public-proxy, readonly, workspace-off build and tests.

#### Scenario: Source integration is not Go module release evidence
- **WHEN** integrated candidate source passes but the selected middleware module fails standalone
- **THEN** source success SHALL NOT authorize its module publication

### Requirement: Gateway container release evidence

Gateway SHALL be a container-only supported release component. Image publication/deployment SHALL require successful same-revision workspace-source multiarchitecture image validation and native smoke, not standalone Gateway readiness against older published pins.

#### Scenario: Gateway container release evidence
- **WHEN** Gateway passes same-revision workspace-source multiarchitecture validation and native smoke but fails against older pins
- **THEN** standalone failure SHALL NOT alone prevent its container publication/deployment under the required same-revision gates

### Requirement: Scheduled pinned-version upgrade and assessment

Configured daily automation SHALL execute pinned-version upgrade and comprehensive assessment, not just discovery. Authorization SHALL include needed compatibility corrections, actionable follow-up issue registration, signed commits, branch push and draft upgrade PR creation. Local memory SHALL be advisory, not source or approval authority. Run-specific assessment/issue links SHALL live in the PR, not dated coverage-map sections or issue catalogs.

#### Scenario: A newer target is available
- **WHEN** no existing pinned-version upgrade PR is active and tooling selects a newer eligible coherent set
- **THEN** the automation SHALL update and validate the reference, comprehensively assess current Go behavior, register remaining parity work and publish a draft PR when its completion conditions are met

#### Scenario: Related upgrade work already exists
- **WHEN** an open PR already advances the pinned reference, even to an older target
- **THEN** the automation SHALL report it and stop rather than create competing work or replace its target

#### Scenario: Nonblocking parity work is registered
- **WHEN** the assessment identifies a remaining actionable difference
- **THEN** the automation SHALL reuse a matching issue or create a work-package issue and link it from the upgrade PR
- **AND** it SHALL update the coverage map only for a durable coverage, evidence, support-boundary or accepted-deviation change
- **AND** registration SHALL NOT authorize implementation of that package or silently accept a permanent deviation

#### Scenario: No version update is needed
- **WHEN** the selected package set matches the registered baseline
- **THEN** the automation SHALL report no upgrade needed without creating an upgrade PR

#### Scenario: Workflow or upgrade is blocked
- **WHEN** tooling is unavailable or completion requires an unavailable published dependency or unresolved material decision
- **THEN** the automation SHALL preserve and report the blocker without weakening checks or claiming success

### Requirement: Scheduled automation authorization boundaries

Automation SHALL NOT automatically implement nonblocking parity packages, approve unresolved material API/scope decisions, merge PRs, mutate shared upstream checkouts or replace another upgrade's fixed target.

#### Scenario: Scheduled automation authorization boundaries
- **WHEN** automation encounters unresolved material API decisions during an upgrade
- **THEN** it SHALL preserve/report the blocker rather than approve it or automatically implement nonblocking packages, merge, mutate shared upstream source or replace another fixed target

### Requirement: ProviderWire V4 contract is a registered parity consumer

The repository SHALL register `ai-gateway/test/providerwire-v4` as a parity TypeScript consumer under `test/conformance/upstream.yaml`. Baseline validation SHALL compare every workspace `ai`/`@ai-sdk/*` dependency with the manifest. Standard parity check SHALL run its non-mutating compile-time surface, production-schema, semantic-golden and registered-client consumption checks.

#### Scenario: ProviderWire workspace matches the baseline
- **WHEN** `ai-gateway/test/providerwire-v4/package.json` pins the registered AI SDK package versions
- **THEN** baseline validation SHALL pass for that consumer

#### Scenario: ProviderWire workspace drifts from the baseline
- **WHEN** an `ai` or `@ai-sdk/*` dependency in `ai-gateway/test/providerwire-v4/package.json` differs from or is absent in `test/conformance/upstream.yaml`
- **THEN** baseline validation SHALL fail and identify the workspace, package, declared version, and baseline version or omission

#### Scenario: Standard parity check includes ProviderWire contract evidence
- **WHEN** a contributor runs the repository parity check
- **THEN** it SHALL typecheck the exhaustive finite surface witnesses
- **AND** it SHALL compile and test the production request schema
- **AND** it SHALL compare in-memory real-client request captures with committed semantic goldens
- **AND** it SHALL run focused unary, SSE, and non-2xx registered-client consumption probes
- **AND** none of those checks SHALL rewrite tracked files

#### Scenario: Baseline upgrade includes the ProviderWire consumer
- **WHEN** the registered upstream package set is upgraded
- **THEN** the ProviderWire workspace dependency pins SHALL be updated with every other parity consumer
- **AND** compile-time drift, schema drift, request-golden drift, and client-consumption drift SHALL be reviewed before the upgrade is complete
- **AND** request goldens SHALL change only through the explicit ProviderWire golden update workflow

#### Scenario: Coverage map records the evidence boundary
- **WHEN** ProviderWire V4 contract coverage is added or changed
- **THEN** `test/conformance/PARITY.md` SHALL identify the registered-client HTTP projection and consumption behavior that is automated
- **AND** it SHALL state that strict Go request replay, server response correctness, runtime lifecycle, privacy, and resource bounds are not established by this contract workspace

### Requirement: ProviderWire contract evidence classification

The parity coverage map SHALL classify ProviderWire contract evidence separately from provider conformance, frontend hook state-machine and future Go ProviderWire runtime coverage.

#### Scenario: ProviderWire contract evidence classification
- **WHEN** ProviderWire workspace contract checks pass
- **THEN** coverage SHALL identify contract evidence separately, not infer provider conformance, hook state-machine or Go runtime coverage
