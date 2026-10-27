## ADDED Requirements

### Requirement: Co-released first-party requirements

Release preparation SHALL set every first-party requirement in every tracked
`go.mod` to the candidate manifest version of the required module, or its
registered initial version if it has never been released, in the same release
pull request. Every first-party requirement of a released library SHALL be an
existing canonical tag or a version released by the same pull request. These
updates SHALL NOT change `go.sum` files and SHALL NOT create release intent.

#### Scenario: Provider uses unreleased root behavior

- **WHEN** root and a provider both have pending releases and the provider uses root behavior that no root tag contains
- **THEN** the release pull request releases both and the provider requires the co-released root version

#### Scenario: Unreleased dependent

- **WHEN** root is released and a dependent library has no pending release
- **THEN** the dependent's requirement is updated to the new root version without releasing the dependent

#### Scenario: Requirement on an unreleased module

- **WHEN** a released library requires a first-party version that is neither tagged nor released by the same pull request
- **THEN** library readiness fails

### Requirement: Post-publication proxy verification

After tagging a library, publication SHALL resolve the new version through the
public Go proxy and checksum database in a fresh cache, with no workspace or
replacements, build a consumer of its public packages and run its deterministic
consumer scenarios. It SHALL query the proxy only after the tag exists. A failure
SHALL be reported for a fix-forward release and SHALL NOT move or delete tags.

#### Scenario: Published library verified

- **WHEN** a library tag is created
- **THEN** an external consumer of that version builds and passes its consumer scenarios through the public proxy

#### Scenario: Published library fails as a consumer

- **WHEN** the new version fails to resolve, build or pass its consumer scenarios through the public proxy
- **THEN** publication reports the failure and the tag is left unchanged

## MODIFIED Requirements

### Requirement: Pull-request-title-derived release intent

The release system SHALL derive each module's release from the Conventional
Commit subject produced by squash-merging the pull request title, without a
separate hand-written intent artifact. `refactor` commits SHALL release the
modules they touch.

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

#### Scenario: Refactor commit

- **WHEN** a squash-merged pull request titled `refactor: ...` touches root source
- **THEN** root is released

### Requirement: Reviewable release pull request

The release system SHALL propose all pending library releases together in one
release pull request that contains each library's calculated version, generated
changelog entries, the updated version manifest and the co-released first-party
requirement updates. Gateway SHALL keep its own release pull request. After
activation, merging a validated release pull request SHALL create one tag and
GitHub Release per released module.

#### Scenario: Pending release

- **WHEN** release-worthy commits for several libraries land on the default branch
- **THEN** one library release pull request is created or refreshed with every pending library's version, changelog and requirement updates

#### Scenario: Pending Gateway release

- **WHEN** release-worthy Gateway changes land on the default branch
- **THEN** Gateway's release is proposed in its own release pull request

#### Scenario: Publication

- **WHEN** the library release pull request is merged
- **THEN** each released library is tagged at the merge commit and receives its own GitHub Release

#### Scenario: No pending release

- **WHEN** no release-worthy commit has landed since the last release
- **THEN** no release pull request is proposed and no tag is created

### Requirement: Artifact-specific release readiness

Ordinary source pull requests and library release pull requests SHALL build and
test every module with same-revision first-party source through local
replacements. A library release pull request's CI SHALL be the behavioral
evidence for the released versions. Readiness SHALL NOT resolve first-party
prerequisites through the public proxy or require a prerequisite tag to contain
the prerequisite's current source.

#### Scenario: Root release while downstream source waits

- **WHEN** root and a provider that uses unreleased root behavior both have pending releases
- **THEN** the release pull request validates both against same-revision source and releases them together

#### Scenario: Stale prerequisite behavior

- **WHEN** a released library's first-party requirement would otherwise lag the prerequisite source it was tested with
- **THEN** release preparation updates the requirement to the version released or already tagged from that source

#### Scenario: Verified release identity and current candidate

- **WHEN** a release manifest version changes
- **THEN** readiness verifies the dedicated App, canonical repository, generated branch, released components, generated-file footprint and exact current-base merge candidate
- **AND** a mutable label alone cannot authorize publication

#### Scenario: Library dependency progression

- **WHEN** root or a provider library publishes
- **THEN** dependent requirements already equal the published version
- **AND** no follow-up requirement update or Renovate pull request is needed

### Requirement: Gateway application release

Gateway SHALL remain independently versioned with changelogs, release pull requests,
and `ai-gateway/vX.Y.Z` application tags. Its main and versioned plugin packages SHALL use
same-revision SDK source through the local replacements in `ai-gateway/go.mod`.
Gateway SHALL NOT wait for library tags or standalone Go compilation against
published versions. Relevant linked SDK changes SHALL contribute Gateway release
intent using release-please mechanisms, without a competing version calculator or
manufactured dependency changes.

#### Scenario: Linked SDK change changes the Gateway image

- **WHEN** a relevant SDK, provider or middleware change affects Gateway plugin behavior built from same-revision source
- **THEN** Gateway can receive application release intent without requiring an ai-gateway path change

#### Scenario: Application release validation

- **WHEN** a Gateway release candidate is prepared
- **THEN** same-revision application tests, plugin builds, plugin validation and protocol test host suites precede any publication
- **AND** library release evidence cannot substitute for that evidence

### Requirement: Publishable module validation

The release system SHALL validate that every published Go module is registered
for release, that its configured tag resolves for the Go tool, and that its only
replacements are local replacements of registered first-party modules.

#### Scenario: Unregistered public module

- **WHEN** a public nested-module `go.mod` exists without a release configuration entry
- **THEN** release validation fails and identifies the missing module

#### Scenario: Unresolvable tag shape

- **WHEN** a nested module's configured tag would not begin with its repository directory
- **THEN** release validation fails

#### Scenario: Local replacement

- **WHEN** a published module replaces a third-party module, a first-party module outside its registered directory, or Gateway
- **THEN** release validation fails

### Requirement: Authentic library release candidate

Library readiness SHALL verify immutable release bot and repository identities,
the changed component versions, matching title/body/manifest versions for every
released component, and an exclusively generated regular-file footprint with
preserved changelog history. The footprint SHALL be the manifest, the released
components' changelogs and `go.mod` files whose only changes are first-party
requirement lines set to the candidate manifest versions. Release-looking PRs
without a manifest update SHALL fail. Ordinary source PRs SHALL retain their
candidate-source gates without requiring publication readiness.

#### Scenario: Inconsistent generated release

- **WHEN** release metadata disagrees with the generated version, changelog or requirement updates of any released component
- **THEN** library readiness fails without authorizing publication

#### Scenario: Release without a version update

- **WHEN** a release-looking PR leaves the version manifest unchanged
- **THEN** it fails rather than being classified as an ordinary source PR

#### Scenario: Combined release body

- **WHEN** the library release pull request releases several components
- **THEN** its body contains exactly one release notes section per released component, each matching that component's manifest version and changelog entry

#### Scenario: Edited module file

- **WHEN** a release pull request changes a `go.mod` line other than a first-party requirement, or changes a `go.sum` file
- **THEN** library readiness fails

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

Authorization SHALL require successful applicable source jobs from the expected canonical workflow at the exact merged revision and authentic PR-readiness evidence bound to the tested head/base/tree and metadata. Missing, expired, stale, skipped, cancelled, failed or unrelated evidence SHALL NOT count as success. Candidate files SHALL NOT replace the trusted authorization tooling.

#### Scenario: Check name spoofing or wrong revision

- **WHEN** a successful check has the right display name but the wrong workflow, repository, event, revision or run identity
- **THEN** it cannot authorize the candidate

#### Scenario: Missing historical readiness evidence

- **WHEN** a merged release has no verifiable retained readiness record
- **THEN** it is blocked and requires an explicit later recovery disposition

#### Scenario: Fresh library prerequisite validation

- **WHEN** authorization evaluates a library
- **THEN** each of its first-party requirements is an existing canonical tag at an ancestor of the merge revision or a component released by the same merge
- **AND** the per-module source jobs at the merged revision cover that library

### Requirement: Independent library authorization decisions

Authorization SHALL report each unambiguous library candidate separately. A selected library's own failure SHALL block that candidate and every library of the same release that requires it, directly or transitively; it SHALL NOT block other ready candidates. Existing candidate-source checks SHALL remain required. Gateway SHALL receive no library authorization.

#### Scenario: Root ready and provider blocked

- **WHEN** root passes authorization while a provider of the same release fails
- **THEN** root is approved in the report and the provider is blocked with its own reason
- **AND** libraries requiring the blocked provider are blocked with the provider as reason

#### Scenario: Gateway is selected

- **WHEN** a Gateway candidate is discovered or explicitly selected
- **THEN** it is identified as unsupported by this library authorization mode

### Requirement: Bounded library publication

Publication SHALL use only freshly authorized candidates from one frozen inventory. Each mutation SHALL match the selected candidate's immutable tag, revision and generated release metadata. Publication SHALL NOT rescan for additional candidates or accept a saved report as authorization. A library SHALL be tagged only after every co-released prerequisite was tagged. Unrelated blocked candidates SHALL retain independent outcomes.

#### Scenario: Late candidate arrival

- **WHEN** another candidate arrives after discovery
- **THEN** publication does not include it in the current batch

#### Scenario: Evidence changes before a write

- **WHEN** PR metadata, required evidence, ancestry or expected tag state changes
- **THEN** the candidate's next mutation is blocked

#### Scenario: Unrelated provider is blocked

- **WHEN** a provider is blocked but root validates
- **THEN** root proceeds to its bounded publication operations

#### Scenario: Co-released prerequisite fails to publish

- **WHEN** a co-released prerequisite cannot be tagged in the current run
- **THEN** its dependents are not tagged and a later run retries them after the prerequisite publishes

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

## REMOVED Requirements

### Requirement: Canonical prerequisite tag identity

**Reason**: Released libraries no longer resolve first-party prerequisites through the public proxy before release; co-released and tagged requirements are checked against the release itself.

**Migration**: See "Co-released first-party requirements" and "Post-publication proxy verification".
