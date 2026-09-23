## MODIFIED Requirements

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
