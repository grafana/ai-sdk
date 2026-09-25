## ADDED Requirements

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
only when its pull request is merged.

#### Scenario: Pending release
- **WHEN** release-worthy commits land on the default branch
- **THEN** one release pull request per pending module is created or refreshed with that module's version

#### Scenario: Publication
- **WHEN** the release pull request is merged
- **THEN** that module is tagged and its GitHub Release is created

#### Scenario: No pending release
- **WHEN** no release-worthy commit has landed since the last release
- **THEN** no release pull request is proposed and no tag is created

### Requirement: Verifiable module resolution across releases
The release system SHALL NOT introduce a state on the default branch in which a
published module requires an unpublished version of another module.

#### Scenario: Core requirement update
- **WHEN** a core release is published
- **THEN** nested modules are repointed at the published core version in a follow-up pull request that updates `go.mod` and `go.sum` together

#### Scenario: Nested first-party requirement update
- **WHEN** OpenAI, Anthropic, OpenAI-compatible, Agent Observability, Logger, or Prometheus is published
- **THEN** dependent modules are promptly repointed at the published version in a follow-up pull request that updates `go.mod` and `go.sum` together

#### Scenario: Multi-level release order
- **WHEN** releases are pending for modules across multiple dependency levels
- **THEN** root is published before every nested module, OpenAI before Bedrock, and Anthropic, OpenAI-compatible, Agent Observability, Logger, and Prometheus before AI Gateway

#### Scenario: Release pull request resolution
- **WHEN** the release pull request is validated by CI
- **THEN** every published module still resolves against the module proxy

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
