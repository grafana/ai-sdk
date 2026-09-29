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

Publication SHALL remain disabled until artifact-specific readiness, required-check
protection, current-candidate authorization, App prerequisites and Gateway release
attribution are reviewed. The activation change SHALL account for already-merged
pending release pull requests. Published tags SHALL remain immutable.

#### Scenario: Incomplete Gateway integration

- **WHEN** workspace image validation or linked release attribution is incomplete
- **THEN** Gateway release readiness fails and automatic tag creation remains disabled

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
