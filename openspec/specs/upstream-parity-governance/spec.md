# upstream-parity-governance Specification

## Purpose

Define repository standards for declaring, checking, upgrading, and reviewing
parity with the upstream Vercel AI SDK baseline.

## Requirements

### Requirement: Upstream parity baseline manifest
The repository SHALL define a checked-in upstream parity baseline manifest that records the verified upstream AI SDK target for parity-sensitive work. The manifest SHALL include the upstream repository URL, the coherent TypeScript package versions used by all parity consumers and reference tooling, the verification status, the verification commands, and known intentional deviations or accepted gaps. Directly consumed packages and transitive packages used as authoritative parity references SHALL be registered so compatibility and upgrade workflows can enforce one coherent package set.

#### Scenario: Contributor finds the verified upstream target
- **WHEN** a contributor needs to know which upstream AI SDK version this repository is verified against
- **THEN** the upstream parity baseline manifest identifies the upstream repository and package versions used for verification

#### Scenario: Baseline records known gaps
- **WHEN** the repository intentionally diverges from upstream behavior or lacks coverage for an upstream surface
- **THEN** the upstream parity baseline manifest records the deviation or gap with enough context for reviewers to classify it as intentional

### Requirement: Parity coverage map
The repository SHALL maintain a stable parity coverage map that classifies compatibility surfaces by verification status, layer, confidence source, durable support boundary, and accepted deviation. The coverage map SHALL cover at minimum core orchestration and UI chunk behavior, provider contract compatibility, provider implementation behavior, frontend interop, and conformance harness capabilities. Each surface SHALL be classified as automated, manual, documented deviation, mixed coverage, or gap. The coverage map SHALL NOT serve as a version-upgrade ledger or actionable-work queue: it SHALL NOT contain dated target assessments, catalogs of `upstream-sync` issues, or copies of issue behavior, impact, scope, dependencies, acceptance criteria, or state.

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
The pinned-version upgrade SHALL account for all assessed differences through upgrade corrections, `upstream-sync` issues or explicit adaptation/exclusion dispositions. One issue SHALL represent a coherent actionable remaining work package, not each upstream commit or assertion, and SHALL be the authoritative durable record of its behavior, exact upstream reference, current Go difference and impact, intended outcome, design decisions, dependencies and acceptance tests. Registration SHALL search by provider/capability/behavior across labeled and unlabeled issues and inspect open and closed candidate matches, including scope, acceptance criteria and resolution. Covered open work SHALL be reused; ambiguous overlap or rejected work SHALL NOT automatically produce a duplicate or reopened issue. The upgrade PR SHALL identify the target, included corrections, validation and a compact exact list of created or reused issues. Repository coverage records SHALL NOT mirror actionable issue content or issue state, and SHALL change only for a durable coverage, evidence, support-boundary or accepted-deviation change. Registration SHALL NOT imply approval of a new API or acceptance of a permanent deviation.

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

### Requirement: Parity issue labels
Every newly created or reused parity work-package issue SHALL carry `upstream-sync`, verified after creation or reuse. Missing required labeling SHALL leave registration incomplete. Optional labels SHALL come from the existing repository inventory and describe the actual work: bug for incorrect existing behavior, enhancement for capabilities/coverage improvements, documentation for documentation-only work, and question for concrete unresolved investigations. Registration SHALL preserve existing labels and SHALL NOT invent labels, infer assignees or automatically apply triage, release, automerge, severity or unrelated workflow labels. Security findings SHALL follow the repository's private reporting policy rather than ordinary public issue registration.

#### Scenario: New parity work is registered
- **WHEN** an actionable work package has no covering issue after duplicate checks
- **THEN** its new issue SHALL include upstream-sync and appropriate existing optional labels
- **AND** its title SHALL identify the area/observable behavior and its body SHALL contain evidence, scope, dependencies and acceptance tests

#### Scenario: Required label cannot be applied
- **WHEN** upstream-sync is unavailable or the label operation fails
- **THEN** registration SHALL report the blocker rather than silently publish the work as fully registered

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
The configured daily parity automation SHALL execute the repository's pinned-version upgrade and comprehensive assessment workflow, not only discover releases. Its authorization SHALL include necessary compatibility corrections, registration of actionable follow-up issues, signed commits, branch push and creation of a draft upgrade PR. It SHALL NOT automatically implement nonblocking parity packages, approve unresolved material API/scope decisions, merge a PR, mutate a shared upstream checkout or replace another upgrade's fixed target. Local memory SHALL be advisory rather than source or approval authority. The automation SHALL keep run-specific assessment and issue links in the upgrade PR and SHALL NOT create a dated assessment section or issue catalog in the parity coverage map.

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

### Requirement: ProviderWire V4 contract is a registered parity consumer

The repository SHALL register `ai-gateway/test/providerwire-v4` as a parity TypeScript consumer governed by `test/conformance/upstream.yaml`. Baseline validation SHALL compare every `ai` and `@ai-sdk/*` dependency in that workspace with the manifest. The standard parity check SHALL run the workspace's non-mutating compile-time surface, production-schema, semantic-golden, and registered-client consumption checks. The parity coverage map SHALL classify this evidence separately from provider conformance, frontend hook state-machine coverage, and future Go ProviderWire runtime coverage.

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
