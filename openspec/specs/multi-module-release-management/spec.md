# Multi-Module Release Management

## Purpose

Define independent, reviewable, and verifiable release management for the AI SDK
library modules and the Gateway application, from merged commits through
artifact-specific readiness, published tags and GitHub Releases.

## Requirements

### Requirement: Pull-request-title-derived release intent

The release system SHALL derive each module's release from the Conventional
Commit subject produced by squash-merging the pull request title, without a
separate hand-written intent artifact.

#### Scenario: Fix commit in one module

- **WHEN** a pull request titled `fix: ...` touches only `providers/openai` and is squash-merged
- **THEN** only the OpenAI provider is released, with a patch-level bump

#### Scenario: Cross-module change

- **WHEN** a squash-merged pull request touches both the root module and a provider
- **THEN** both modules are released from that commit

#### Scenario: Non-releasing change

- **WHEN** the squash-merged pull request title uses a non-releasing type such as `chore`, `ci`, or `test`
- **THEN** no module is released

#### Scenario: Unparsable pull request title

- **WHEN** a pull request title is not a Conventional Commit
- **THEN** a required check fails before the pull request can reach the release history

### Requirement: Independent Go module versions

The release system SHALL track each published module's version independently and
SHALL generate tag names that the Go tool can resolve for the module's
repository directory.

#### Scenario: Root module tag

- **WHEN** the root module is released
- **THEN** its tag has the form `vX.Y.Z`

#### Scenario: Nested module tag

- **WHEN** a provider or middleware module is released
- **THEN** its tag has the form `<module-directory>/vX.Y.Z`

#### Scenario: Never-released module

- **WHEN** a registered module has no recorded version and no existing tag
- **THEN** its first release uses the registered initial version

### Requirement: Reviewable release pull request

The release system SHALL propose every pending module release as a separate pull
request that contains the calculated version, generated changelog entries, and
updated version manifest, and SHALL create that module's tag and GitHub Release
only after activation when its validated pull request is merged.

#### Scenario: Pending release

- **WHEN** release-worthy commits land on the default branch
- **THEN** one release pull request per pending module is created or refreshed with that module's version

#### Scenario: Publication

- **WHEN** the release pull request is merged
- **THEN** that module is tagged and its GitHub Release is created

#### Scenario: No pending release

- **WHEN** no release-worthy commit has landed since the last release
- **THEN** no release pull request is proposed and no tag is created

### Requirement: Artifact-specific release readiness

Ordinary source pull requests SHALL validate coordinated candidate source and merged
internal pins without requiring every module to compile standalone. Library release
pull requests SHALL separately validate the selected module and actual dependency
closure against canonical tagged prerequisites with readonly standalone checks and a
fresh public-proxy cache. Prerequisite owned source SHALL cover the current candidate
base; old dependency compilation alone SHALL NOT establish readiness.

#### Scenario: Root release while downstream source waits

- **WHEN** root is ready and a downstream provider needs an unreleased prerequisite
- **THEN** the root release check validates root without requiring the unrelated provider to pass standalone

#### Scenario: Stale prerequisite behavior

- **WHEN** a pinned library prerequisite omits owned runtime source changes in the current base
- **THEN** the dependent library release fails readiness even if it compiles

#### Scenario: Verified release identity and current candidate

- **WHEN** a release manifest version changes
- **THEN** readiness verifies the dedicated App, canonical repository, generated branch, single component, generated-file footprint and exact current-base merge candidate
- **AND** a mutable label alone cannot authorize publication

#### Scenario: Library dependency progression

- **WHEN** root or a provider library publishes
- **THEN** Renovate advances library requirements in follow-up source pull requests
- **AND** root and intermediate dependency updates do not require unrelated downstream modules to pass standalone

### Requirement: Gateway application release

Gateway SHALL remain independently versioned with changelogs, release pull requests,
and `ai-gateway/vX.Y.Z` application tags. Its main and versioned images SHALL use the
workspace at the selected revision. Gateway SHALL NOT wait for library tags, repins,
or standalone Go compilation. Relevant linked workspace changes SHALL contribute
Gateway release intent using release-please mechanisms, without a competing version
calculator or manufactured dependency changes.

#### Scenario: Linked SDK change changes the Gateway image

- **WHEN** a relevant SDK, provider or middleware change affects workspace-built Gateway behavior
- **THEN** Gateway can receive application release intent without requiring an ai-gateway path change

#### Scenario: Application release validation

- **WHEN** a Gateway release candidate is prepared
- **THEN** same-revision application tests, image builds and runtime smoke validation precede publication
- **AND** library standalone readiness cannot substitute for that evidence

### Requirement: Fail-closed activation

Release automation, including release PR preparation, SHALL remain disabled until
artifact-specific readiness, required-check protection, current-candidate
authorization, App prerequisites and Gateway release
attribution are reviewed. The activation change SHALL account for already-merged
pending release pull requests. Published tags SHALL remain immutable.

#### Scenario: Incomplete Gateway integration

- **WHEN** workspace image validation or linked release attribution is incomplete
- **THEN** Gateway release readiness fails and release PR preparation and
  automatic tag creation remain disabled

### Requirement: Publishable module validation

The release system SHALL validate that every published Go module is registered
for release, that its configured tag resolves for the Go tool, and that it
contains no local filesystem replacement.

#### Scenario: Unregistered public module

- **WHEN** a public nested-module `go.mod` exists without a release configuration entry
- **THEN** release validation fails and identifies the missing module

#### Scenario: Unresolvable tag shape

- **WHEN** a nested module's configured tag would not begin with its repository directory
- **THEN** release validation fails

#### Scenario: Local replacement

- **WHEN** a published module contains a local `replace` directive
- **THEN** release validation fails

### Requirement: Prerelease lifecycle

The release system SHALL keep releases in a configured prerelease channel and
SHALL graduate to stable versions only through a reviewed configuration change.

#### Scenario: Continue the alpha channel

- **WHEN** the current version is `v0.1.0-alpha.1` and a release-worthy commit lands
- **THEN** the next version is `v0.1.0-alpha.2` and its GitHub Release is marked as a prerelease

#### Scenario: Graduate to stable

- **WHEN** the prerelease setting is disabled and a release-worthy commit lands
- **THEN** the next version drops the prerelease suffix

### Requirement: Authentic library release candidate

Library readiness SHALL verify immutable release bot and repository identities,
one component version, matching title/body/manifest versions, and an exclusively
generated regular-file footprint with preserved changelog history. Release-looking
PRs without a manifest update SHALL fail. Ordinary source PRs SHALL retain their
candidate-source gates without requiring publication readiness.

#### Scenario: Inconsistent generated release

- **WHEN** release metadata disagrees with the generated version or changelog
- **THEN** library readiness fails without authorizing publication

#### Scenario: Release without a version update

- **WHEN** a release-looking PR leaves the version manifest unchanged
- **THEN** it fails rather than being classified as an ordinary source PR

### Requirement: Current library candidate validation

Library readiness SHALL require a clean checkout merging the exact live release
head and current canonical base. It SHALL recheck live metadata and canonical main
after validation and reject changes during the run. A successful check SHALL NOT
replace required-check/current-base protection or post-merge authorization.

#### Scenario: Main advances during validation

- **WHEN** canonical main advances while library readiness runs
- **THEN** the stale candidate fails the final current-base check

#### Scenario: Candidate changes during validation

- **WHEN** the release head or release metadata changes while readiness runs
- **THEN** the run fails and requires validation of the new candidate

### Requirement: Canonical prerequisite tag identity

Library prerequisites SHALL resolve from the public proxy without replacements,
use tagged versions, match canonical Git origin and canonical tag targets, and
be ancestors of the candidate base. Missing or unverifiable evidence SHALL fail
readiness before standalone or consumer success can authorize a library.

#### Scenario: Published version has the wrong tag target

- **WHEN** a prerequisite download origin differs from its canonical tag target
- **THEN** library readiness rejects the prerequisite

#### Scenario: Prerequisite cannot be verified

- **WHEN** a prerequisite is a pseudo-version, unmerged, replaced, unavailable,
  or has an unverifiable origin
- **THEN** library readiness fails closed

### Requirement: Frozen merged-library candidate inventory

Authorization SHALL operate on a complete, frozen inventory of pending merged library releases. Each record SHALL identify the canonical repository, component, version, tag, PR, release head, merge revision, merge parent, tree and generated metadata. Discovery failures, ambiguous candidates and unsupported components SHALL NOT yield authorization. New arrivals SHALL require a later inventory.

#### Scenario: Candidate arrives during validation

- **WHEN** another release PR merges after the inventory is frozen
- **THEN** it is absent from the current authorization results and cannot enter the approved set

#### Scenario: Partial discovery

- **WHEN** candidate discovery fails before all requested pages are read
- **THEN** the run fails without emitting an approved set from the partial inventory

#### Scenario: Conflicting candidates

- **WHEN** multiple pending candidates claim the same component or tag
- **THEN** those candidates are blocked with an explicit ambiguity reason

### Requirement: Authentic merged-library identity and tree

Authorization SHALL verify immutable release App and repository identity, a signed release head, canonical main ancestry, one generated component change and consistent version/notes/tag metadata. The supported squash revision SHALL have one parent and the same complete tree as the successfully validated PR candidate; its parent SHALL match that candidate's validated base. Missing proof SHALL block authorization.

#### Scenario: Merge bypass includes new source

- **WHEN** a release merges after its validated base becomes stale, or its merged tree differs from the validated candidate tree
- **THEN** authorization fails even if the old PR checks succeeded

#### Scenario: Post-merge metadata edit

- **WHEN** the PR title or release body no longer matches retained validated metadata and merged generated files
- **THEN** authorization fails instead of constructing new release contents from the edited metadata

#### Scenario: Delayed authorization

- **WHEN** newer commits land after a valid release squash merge
- **THEN** authorization validates the frozen merge and its parent rather than substituting current main
- **AND** the merge must remain reachable from canonical main

#### Scenario: Unsupported merge history

- **WHEN** merge topology or retained evidence cannot establish the supported squash candidate identity
- **THEN** the candidate is blocked without modifying history or repository protections

### Requirement: Revision-bound source and readiness evidence

Authorization SHALL require successful applicable source jobs from the expected canonical workflow at the exact merged revision, authentic PR-readiness evidence bound to the tested head/base/tree and metadata, and fresh selected-library validation at the merged revision. Missing, expired, stale, skipped, cancelled, failed or unrelated evidence SHALL NOT count as success. Candidate files SHALL NOT replace the trusted authorization tooling.

#### Scenario: Check name spoofing or wrong revision

- **WHEN** a successful check has the right display name but the wrong workflow, repository, event, revision or run identity
- **THEN** it cannot authorize the candidate

#### Scenario: Missing historical readiness evidence

- **WHEN** a merged release has no verifiable retained readiness record
- **THEN** it is blocked and requires an explicit later recovery disposition

#### Scenario: Fresh library prerequisite validation

- **WHEN** authorization evaluates a library
- **THEN** its tagged prerequisite origins, ancestry and owned-source freshness are revalidated against its immutable merge parent
- **AND** standalone and consumer checks use the selected merged library with published prerequisites

### Requirement: Independent library authorization decisions

Authorization SHALL report each unambiguous library candidate separately. A selected library's actual prerequisite failure SHALL block that candidate; unrelated downstream standalone failures SHALL NOT block a ready candidate. Existing candidate-source checks SHALL remain required. Gateway SHALL receive no library authorization.

#### Scenario: Root ready and provider blocked

- **WHEN** root passes its merged-source and selected-library checks while a provider requires an unreleased root prerequisite
- **THEN** root is approved in the report and the provider is blocked with its own reason
- **AND** root validation does not invoke the provider's standalone checks

#### Scenario: Gateway is selected

- **WHEN** a Gateway candidate is discovered or explicitly selected
- **THEN** it is identified as unsupported by this library authorization mode

### Requirement: Read-only authorization boundary

The authorization mode SHALL perform no GitHub mutations, tag writes, release publication or activation changes. Its report SHALL identify the immutable candidates, evidence and tooling revision, and state that it grants no standing publication permission. Changed metadata, evidence or tag state SHALL invalidate a previous decision; future publication SHALL revalidate rather than trust an editable report.

#### Scenario: Tag already exists

- **WHEN** a candidate tag or corresponding release already exists, or its state cannot be verified
- **THEN** this slice blocks authorization and reports that publication recovery is outside its scope

#### Scenario: Evidence changes during validation

- **WHEN** candidate metadata, relevant run attempt or tag state changes before the final recheck
- **THEN** the affected candidate is not approved

#### Scenario: Attempted publication

- **WHEN** the authorization driver attempts a remote mutation or publication API
- **THEN** the transport rejects it and no tag, release, label or PR mutation occurs
