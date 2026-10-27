## MODIFIED Requirements

### Requirement: Reviewable release pull request

The release system SHALL propose all pending library releases together in one
release pull request that contains each library's calculated version, generated
changelog entries and the updated version manifest. Gateway SHALL keep its own
release pull request. A library configured with `separate-pull-requests: true`
SHALL receive its own release pull request. After activation, merging a validated
release pull request SHALL create one tag and GitHub Release per released module.

#### Scenario: Pending release

- **WHEN** release-worthy commits for several libraries land on the default branch
- **THEN** one library release pull request is created or refreshed with every pending library's version and changelog

#### Scenario: Pending Gateway release

- **WHEN** release-worthy Gateway changes land on the default branch
- **THEN** Gateway's release is proposed in its own release pull request

#### Scenario: Publication

- **WHEN** the library release pull request is merged
- **THEN** each released library is tagged and receives its own GitHub Release

#### Scenario: No pending release

- **WHEN** no release-worthy commit has landed since the last release
- **THEN** no release pull request is proposed and no tag is created

### Requirement: Artifact-specific release readiness

Ordinary source pull requests SHALL validate coordinated candidate source and merged
internal pins without requiring every module to compile standalone. Library release
pull requests SHALL validate every released library and its actual dependency
closure against canonical tagged prerequisites with readonly standalone checks and a
fresh public-proxy cache. A pinned prerequisite tag SHALL NOT need to contain the
prerequisite's current source.

#### Scenario: Stale prerequisite behavior

- **WHEN** a released library pins a canonical prerequisite tag that predates prerequisite source changes in the current base
- **AND** the library passes its standalone and consumer checks against that tag
- **THEN** readiness accepts the library

#### Scenario: Root release while downstream source waits

- **WHEN** a released library fails its standalone checks because it needs unreleased prerequisite source
- **THEN** readiness fails the release pull request
- **AND** configuring that library with `separate-pull-requests: true` moves it to its own release pull request so the others can release

#### Scenario: Verified release identity and current candidate

- **WHEN** a release manifest version changes
- **THEN** readiness verifies the dedicated App, canonical repository, generated branch, released components, generated-file footprint and exact current-base merge candidate
- **AND** a mutable label alone cannot authorize publication

#### Scenario: Library dependency progression

- **WHEN** root or a provider library publishes
- **THEN** Renovate advances library requirements in follow-up source pull requests
- **AND** root and intermediate dependency updates do not require unrelated downstream modules to pass standalone

### Requirement: Authentic library release candidate

Library readiness SHALL verify immutable release bot and repository identities,
the changed component versions, matching title/body/manifest versions for every
released component, and an exclusively generated regular-file footprint of the
manifest and the released components' changelogs with preserved changelog
history. Separately released components SHALL NOT share a release pull request.
Release-looking PRs without a manifest update SHALL fail. Ordinary source PRs
SHALL retain their candidate-source gates without requiring publication readiness.

#### Scenario: Inconsistent generated release

- **WHEN** release metadata disagrees with the generated version or changelog of any released component
- **THEN** library readiness fails without authorizing publication

#### Scenario: Release without a version update

- **WHEN** a release-looking PR leaves the version manifest unchanged
- **THEN** it fails rather than being classified as an ordinary source PR

#### Scenario: Combined release body

- **WHEN** the library release pull request releases several components
- **THEN** its body contains exactly one release notes section per released component, each matching that component's manifest version and changelog entry

### Requirement: Authentic merged-library identity and tree

Authorization SHALL verify immutable release App and repository identity, a signed release head, canonical main ancestry, the generated component changes and consistent version/notes/tag metadata for the selected component. The supported squash revision SHALL have one parent and the same complete tree as the successfully validated PR candidate; its parent SHALL match that candidate's validated base. Missing proof SHALL block authorization.

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

#### Scenario: Component outside the validated release

- **WHEN** a discovered candidate's component or version is absent from the retained readiness evidence
- **THEN** authorization blocks that candidate

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
- **THEN** its tagged prerequisite origins and ancestry are revalidated against its immutable merge parent
- **AND** standalone and consumer checks use the selected merged library with published prerequisites

### Requirement: Immutable library publication recovery

Publication retries SHALL revalidate the original candidate and inspect canonical tag and release state. Matching partial publication MAY finish without modifying existing tags or releases. Conflicting or unverifiable state SHALL block writes. Completed releases SHALL not be recreated; lifecycle completion SHALL remain discoverable after partial failure. A release pull request's lifecycle labels SHALL move only after every library it released is published.

#### Scenario: Tag created and release failed

- **WHEN** a previous attempt created the exact candidate tag but no release
- **THEN** retry creates only the missing release after fresh validation
- **AND** the tag is never moved

#### Scenario: Release exists with incomplete labels

- **WHEN** the exact candidate release exists but lifecycle labeling failed
- **THEN** retry completes labels without recreating or editing the release

#### Scenario: Conflicting existing tag or metadata

- **WHEN** a tag resolves elsewhere or release metadata disagrees with the candidate
- **THEN** publication and recovery fail without modifying that state

#### Scenario: One library of a combined release is blocked

- **WHEN** some libraries of a merged release pull request publish and another is blocked
- **THEN** the pull request keeps its pending label so a later run rediscovers the blocked library
- **AND** the published libraries are recovered without new writes on that run
