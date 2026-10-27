# Spec Delta

## ADDED Requirements

### Requirement: Release-coupled first-party requirement updates

The combined library release pull request SHALL update first-party requirements only in the modules it releases and in Gateway. Each such requirement SHALL name the version released in the same pull request, or otherwise the latest tagged version. Modules that are not released SHALL keep their manifests unchanged. Dependency automation SHALL NOT propose first-party requirement updates.

#### Scenario: Root and Bedrock released together

- **WHEN** root and Bedrock are pending and OpenAI is not
- **THEN** Bedrock requires the root version released in that pull request and the latest tagged OpenAI version
- **AND** the OpenAI manifest is unchanged

#### Scenario: Gateway pins follow co-released libraries

- **WHEN** the combined pull request releases libraries that Gateway requires
- **THEN** Gateway requires the co-released versions without receiving release intent from that change

#### Scenario: Dependency automation

- **WHEN** a first-party module publishes a new version
- **THEN** no dependency-update pull request proposes that version to another module

### Requirement: Candidate module checksums

Checksums recorded for co-released first-party versions SHALL be computed from module archives built from the exact candidate tree, in dependency order, with an isolated module cache. Those archives SHALL be byte-identical in content hash to the archives the public Go proxy serves for the resulting tags.

#### Scenario: Recorded checksum matches publication

- **WHEN** a co-released module's tag is published and fetched from the public proxy
- **THEN** its module and `go.mod` hashes equal the entries recorded in the dependents' `go.sum`

#### Scenario: Stale cached candidate

- **WHEN** a module cache already holds an archive for a candidate version
- **THEN** checksum computation does not use it

### Requirement: Post-publication proxy verification

After library tags are created, publication SHALL fetch each published version from the public Go proxy and compare its hashes with the checksums recorded in the release. A mismatch or an unavailable version after bounded retries SHALL fail the run and SHALL NOT move or delete tags.

#### Scenario: Proxy serves a different archive

- **WHEN** the public proxy hash of a published version differs from the recorded checksum
- **THEN** publication fails and reports the module and version
- **AND** the tag is left unchanged for a corrective release

## MODIFIED Requirements

### Requirement: Reviewable release pull request

The release system SHALL propose all pending library module releases in one combined
release pull request that contains each module's calculated version, generated
changelog entries, the updated version manifest and the release-coupled first-party
requirement updates. Gateway SHALL keep its own release pull request. The release
system SHALL create each module's tag and GitHub Release only after activation when
its validated pull request is merged.

#### Scenario: Pending release

- **WHEN** release-worthy commits land on the default branch for several library modules
- **THEN** one combined release pull request is created or refreshed with each pending library's version

#### Scenario: Pending Gateway release

- **WHEN** release-worthy commits land for Gateway
- **THEN** Gateway's release is proposed in its own release pull request, separate from the combined library pull request

#### Scenario: Publication

- **WHEN** the combined release pull request is merged
- **THEN** each released library is tagged at that merge commit and its GitHub Release is created

#### Scenario: No pending release

- **WHEN** no release-worthy commit has landed since the last release
- **THEN** no release pull request is proposed and no tag is created

### Requirement: Artifact-specific release readiness

Ordinary source pull requests SHALL validate coordinated candidate source and merged
internal pins without requiring every module to compile standalone. Library release
pull requests SHALL separately validate each selected module and its actual dependency
closure with readonly standalone checks and a fresh module cache. Already-tagged
prerequisites SHALL resolve from the public proxy; co-released prerequisites SHALL
resolve from candidate archives of the same pull request. Prerequisite owned source
SHALL cover the current candidate base; old dependency compilation alone SHALL NOT
establish readiness.

#### Scenario: Root release while downstream source waits

- **WHEN** root is ready and a downstream provider needs an unreleased prerequisite
- **THEN** the root release check validates root without requiring the unrelated provider to pass standalone

#### Scenario: Stale prerequisite behavior

- **WHEN** a pinned library prerequisite omits owned runtime source changes in the current base and is not released in the same pull request
- **THEN** the dependent library release fails readiness even if it compiles

#### Scenario: Co-released prerequisite

- **WHEN** Bedrock and OpenAI are released in the same pull request
- **THEN** Bedrock's readiness resolves OpenAI from the candidate archive of that pull request
- **AND** OpenAI's owned source is fresh because it is tagged at the same merge commit

#### Scenario: Verified release identity and current candidate

- **WHEN** a release manifest version changes
- **THEN** readiness verifies the dedicated App, canonical repository, generated branch, released components, generated-file footprint and exact current-base merge candidate
- **AND** a mutable label alone cannot authorize publication

#### Scenario: Library dependency progression

- **WHEN** root or a provider library is released
- **THEN** dependent library requirements advance only in the combined release pull request that releases those dependents
- **AND** root and intermediate dependency updates do not require unrelated downstream modules to pass standalone

### Requirement: Gateway application release

Gateway SHALL remain independently versioned with changelogs, release pull requests,
and `ai-gateway/vX.Y.Z` application tags. Its main and versioned plugin packages SHALL use the
workspace at the selected revision. Gateway SHALL NOT wait for library tags, repins,
or standalone Go compilation. Relevant linked workspace changes SHALL contribute
Gateway release intent using release-please mechanisms, without a competing version
calculator or manufactured dependency changes. First-party requirement updates made by
the combined library release pull request SHALL NOT contribute Gateway release intent.

#### Scenario: Linked SDK change changes the Gateway image

- **WHEN** a relevant SDK, provider or middleware change affects workspace-built Gateway plugin behavior
- **THEN** Gateway can receive application release intent without requiring an ai-gateway path change

#### Scenario: Application release validation

- **WHEN** a Gateway release candidate is prepared
- **THEN** same-revision application tests, plugin builds, plugin validation and protocol test host suites precede any publication
- **AND** library standalone readiness cannot substitute for that evidence

#### Scenario: Library release updates Gateway pins

- **WHEN** a merged combined library release pull request changes `ai-gateway/go.mod`
- **THEN** no Gateway release is proposed from that change alone

### Requirement: Authentic library release candidate

Library readiness SHALL verify immutable release bot and repository identities,
one version per released component, matching title/body/manifest versions, and an
exclusively generated regular-file footprint with preserved changelog history. The
generated footprint SHALL be limited to the manifest, released components' changelogs
and the first-party requirement files of released modules and Gateway. Release-looking
PRs without a manifest update SHALL fail. Ordinary source PRs SHALL retain their
candidate-source gates without requiring publication readiness.

#### Scenario: Inconsistent generated release

- **WHEN** release metadata disagrees with the generated version or changelog
- **THEN** library readiness fails without authorizing publication

#### Scenario: Release without a version update

- **WHEN** a release-looking PR leaves the version manifest unchanged
- **THEN** it fails rather than being classified as an ordinary source PR

#### Scenario: Requirement change outside the release

- **WHEN** the release pull request changes a `go.mod` or `go.sum` of a module it does not release, other than Gateway's
- **THEN** library readiness fails

#### Scenario: Non-first-party requirement change

- **WHEN** a generated requirement update changes a third-party requirement that the candidate archives do not imply
- **THEN** library readiness fails

### Requirement: Canonical prerequisite tag identity

Already-tagged library prerequisites SHALL resolve from the public proxy without
replacements, use tagged versions, match canonical Git origin and canonical tag
targets, and be ancestors of the candidate base. Co-released prerequisites SHALL
resolve only from archives built from the candidate tree whose hashes match the
recorded checksums. Missing or unverifiable evidence SHALL fail readiness before
standalone or consumer success can authorize a library.

#### Scenario: Published version has the wrong tag target

- **WHEN** a prerequisite download origin differs from its canonical tag target
- **THEN** library readiness rejects the prerequisite

#### Scenario: Prerequisite cannot be verified

- **WHEN** a prerequisite is a pseudo-version, unmerged, replaced, unavailable,
  or has an unverifiable origin
- **THEN** library readiness fails closed

#### Scenario: Co-released archive does not match

- **WHEN** a co-released prerequisite archive built from the candidate tree does not match the recorded checksum
- **THEN** library readiness fails closed

### Requirement: Frozen merged-library candidate inventory

Authorization SHALL operate on a complete, frozen inventory of pending merged library releases. Each record SHALL identify the canonical repository, component, version, tag, PR, release head, merge revision, merge parent, tree and generated metadata. One merged combined release pull request SHALL yield one record per released component. Discovery failures, ambiguous candidates and unsupported components SHALL NOT yield authorization. New arrivals SHALL require a later inventory.

#### Scenario: Candidate arrives during validation

- **WHEN** another release PR merges after the inventory is frozen
- **THEN** it is absent from the current authorization results and cannot enter the approved set

#### Scenario: Partial discovery

- **WHEN** candidate discovery fails before all requested pages are read
- **THEN** the run fails without emitting an approved set from the partial inventory

#### Scenario: Conflicting candidates

- **WHEN** multiple pending candidates claim the same component or tag
- **THEN** those candidates are blocked with an explicit ambiguity reason

#### Scenario: Combined release pull request

- **WHEN** one merged combined release pull request releases several libraries
- **THEN** the inventory holds one unambiguous record per released library, all sharing that pull request and merge revision

### Requirement: Authentic merged-library identity and tree

Authorization SHALL verify immutable release App and repository identity, a signed release head, canonical main ancestry, the generated changes of the released components and consistent version/notes/tag metadata. The supported squash revision SHALL have one parent and the same complete tree as the successfully validated PR candidate; its parent SHALL match that candidate's validated base. Missing proof SHALL block authorization.

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
- **AND** standalone and consumer checks use the selected merged library with published prerequisites and archives of co-released prerequisites built from the merge revision

### Requirement: Independent library authorization decisions

Authorization SHALL report each unambiguous library candidate separately. A selected library's actual prerequisite failure SHALL block that candidate, including the failure of a co-released prerequisite it requires; unrelated downstream standalone failures SHALL NOT block a ready candidate. Existing candidate-source checks SHALL remain required. Gateway SHALL receive no library authorization.

#### Scenario: Root ready and provider blocked

- **WHEN** root passes its merged-source and selected-library checks while a provider requires an unreleased root prerequisite
- **THEN** root is approved in the report and the provider is blocked with its own reason
- **AND** root validation does not invoke the provider's standalone checks

#### Scenario: Co-released prerequisite blocked

- **WHEN** OpenAI is blocked and Bedrock in the same release requires the co-released OpenAI version
- **THEN** Bedrock is blocked with a reason naming OpenAI
- **AND** libraries that do not require OpenAI keep their own outcomes

#### Scenario: Gateway is selected

- **WHEN** a Gateway candidate is discovered or explicitly selected
- **THEN** it is identified as unsupported by this library authorization mode

### Requirement: Bounded library publication

Publication SHALL use only freshly authorized candidates from one frozen inventory. Each mutation SHALL match the selected candidate's immutable tag, revision and generated release metadata. A candidate SHALL be published only together with every co-released prerequisite it requires. Publication SHALL NOT rescan for additional candidates or accept a saved report as authorization. Unrelated blocked candidates SHALL retain independent outcomes.

#### Scenario: Late candidate arrival

- **WHEN** another candidate arrives after discovery
- **THEN** publication does not include it in the current batch

#### Scenario: Evidence changes before a write

- **WHEN** PR metadata, required evidence, ancestry or expected tag state changes
- **THEN** the candidate's next mutation is blocked

#### Scenario: Unrelated provider is blocked

- **WHEN** a provider cannot validate its prerequisites but root validates
- **THEN** only root may proceed to its bounded publication operations

#### Scenario: Co-released prerequisite fails during publication

- **WHEN** creating a co-released prerequisite's tag fails
- **THEN** libraries requiring that version are not tagged in that run
- **AND** a retry resumes from canonical tag state without moving existing tags
