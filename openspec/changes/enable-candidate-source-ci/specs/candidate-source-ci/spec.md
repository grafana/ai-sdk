## ADDED Requirements

### Requirement: Candidate source is the required integration target
Required source CI SHALL compile, lint, vet, test and run cross-language integration for candidate SDK, provider, middleware and Gateway code together. It SHALL retain formatting, docs, security, parity, conformance, structural boundary, module-policy fixture and merged-pin checks. It SHALL NOT require a published dependency to implement a changed candidate API for ordinary source PR merge eligibility.

#### Scenario: Coordinated change with older merged pins
- **WHEN** a controlled breaking SDK, provider and Gateway change is coherent in the candidate workspace but cannot build standalone against unchanged older merged pins
- **THEN** every required source check SHALL pass on the candidate revision
- **AND** the standalone failure SHALL remain observable outside source eligibility

#### Scenario: ProviderWire differential red controls
- **WHEN** the required ProviderWire client contract and request-mutation controls run
- **THEN** the Go differential SHALL execute the candidate Grafana client against candidate SDK root while retaining the pinned TypeScript comparator
- **AND** each copied client mutant SHALL be selected in an explicit candidate-root workspace and rejected by a semantic differential assertion, not a build or setup failure
- **AND** genuinely pinned Go-client standalone consumption evidence MAY run separately as a nonblocking PR diagnostic without demoting the candidate differential or red controls

#### Scenario: Real source regression
- **WHEN** candidate code fails a required source, parity, integration, formatting, security or policy check
- **THEN** the source PR SHALL be blocked without treating an unrelated standalone diagnostic as the cause

### Requirement: Diagnostic and publication checks are independent
Public-proxy standalone validation SHALL remain available and visible on source PRs without a direct or transitive required-check dependency. Gateway image publication on main or Gateway tags SHALL require successful exact-checkout-revision public-proxy readonly workspace-off standalone validation without local replacements, production image validation and all required source checks. Deployment SHALL require successful publication of that same revision. Skipped, cancelled and failed artifact checks SHALL block publication and deployment, not a source PR.

#### Scenario: Diagnostic failure on a source PR
- **WHEN** a PR's candidate source checks succeed and its standalone diagnostic fails
- **THEN** the diagnostic SHALL report failure clearly, while no required PR aggregate or source job SHALL fail on its account

#### Scenario: Push to main or Gateway tag with unready artifact
- **WHEN** standalone dependency build or image validation fails at the checkout SHA on a canonical main push or `ai-gateway/v*` tag
- **THEN** that revision SHALL NOT publish an image or deploy even if its source checks passed

#### Scenario: Valid artifact
- **WHEN** all required source checks and standalone artifact and image gates pass for the same checkout SHA on an eligible push
- **THEN** image publication MAY proceed; deployment on main SHALL only follow successful publication

#### Scenario: Missing gate result
- **WHEN** an artifact prerequisite is skipped or cancelled on an eligible push
- **THEN** the publisher SHALL NOT treat the absent result as success

### Requirement: Approved rollout and manual module release boundary
Activation SHALL document maintainer-approved changes to required checks/rulesets, without bypasses or orphaned check names. Until #245 release readiness is integrated in #21, maintainers SHALL validate each selected published module standalone before manual publication and SHALL NOT enable old #21 SDK release assumptions unchanged. Candidate-source merge success SHALL NOT authorize automatic SDK tagging.

#### Scenario: Required check identity changes
- **WHEN** job names or required-check identities must change
- **THEN** maintainers SHALL approve the migration and coordinate settings with workflow rollout before the new check graph becomes required

#### Scenario: Module publication before release automation
- **WHEN** a maintainer intends to publish a module after candidate source merges
- **THEN** the maintainer SHALL run selected-module standalone validation before manual publication
- **AND** no source CI success SHALL itself create an SDK tag
