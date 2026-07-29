# Multi-Module Release Management

## Purpose

Define independent, deterministic, and safe release management for the AI SDK
root module and its public nested Go modules, from reviewed change intent through
release preparation and publication.

## Requirements

### Requirement: Explicit release intent

The release system SHALL accept reviewed change fragments that identify one or
more registered modules, a semantic bump level for each module, and a shared
changelog entry.

#### Scenario: Cross-module change uses different bump levels

- **WHEN** a change fragment declares a minor core change and patch provider changes
- **THEN** the release plan assigns the declared bump independently to each module

#### Scenario: Invalid release intent is rejected

- **WHEN** a fragment names an unknown module, unsupported bump, or empty changelog entry
- **THEN** validation fails without modifying repository files

### Requirement: Independent Go module versions

The release system SHALL derive each module's current version from immutable Git
tags and SHALL generate tag names compatible with the module's repository
directory.

#### Scenario: Root module tag

- **WHEN** the core module is planned for release
- **THEN** its tag has the form `vX.Y.Z`

#### Scenario: Nested module tag

- **WHEN** a provider or middleware module is planned for release
- **THEN** its tag has the form `<module-directory>/vX.Y.Z`

#### Scenario: Unreleased module

- **WHEN** a registered module has no existing version tag
- **THEN** the plan uses the registry's initial version as its next release

### Requirement: Deterministic release planning

The release system SHALL aggregate all pending fragments deterministically,
select the highest requested bump per module, calculate the next versions, and
order modules according to internal dependencies without mutating files.

#### Scenario: Multiple fragments affect one module

- **WHEN** pending fragments request patch and minor bumps for the same module
- **THEN** the plan selects a minor bump and retains both changelog entries

#### Scenario: Repeated plan

- **WHEN** the same repository state is planned more than once
- **THEN** the rendered plan and machine-readable plan data are identical

### Requirement: Selective module releases

The release system SHALL allow planning and preparation to select one or more
registered modules without forcing unrelated pending modules into the release.

#### Scenario: Select one pending module

- **WHEN** core and OpenAI provider intent are pending and only the OpenAI provider is selected
- **THEN** the plan contains only the OpenAI provider release

#### Scenario: No module selector

- **WHEN** planning or preparation runs without a module selector
- **THEN** all pending module intent is included

#### Scenario: Shared fragment is partially consumed

- **WHEN** one fragment names selected and deferred modules
- **THEN** preparation consumes the selected entries and preserves the deferred entries with the original summary

#### Scenario: Selected module has no pending intent

- **WHEN** a registered module is selected without a pending bump
- **THEN** planning fails without modifying release files

### Requirement: Reviewable release preparation

The release system SHALL prepare a release as repository changes that update
selected changelogs, update declared internal root requirements, consume the
included fragment entries, preserve deferred entries, and write an exact
release-plan manifest.

#### Scenario: Prepare dependent module with core

- **WHEN** core and a dependent provider are prepared together
- **THEN** the provider requirement names the planned core version and both changelogs are updated

#### Scenario: Preparation failure

- **WHEN** preparation encounters inconsistent tags, invalid metadata, or an uneditable module file
- **THEN** it fails before publication and reports the inconsistent input

### Requirement: Safe module publication

The release system SHALL default publication to a dry run and SHALL require
explicit confirmation before creating or pushing tags or creating GitHub
Releases.

#### Scenario: Publication without confirmation

- **WHEN** publication is invoked without its confirmation flag
- **THEN** it reports the intended checks and external mutations without performing them

#### Scenario: Dependency-ordered publication

- **WHEN** core and dependent modules are published together
- **THEN** the core tag is pushed before dependents are tested with `GOWORK=off` and tagged

#### Scenario: Idempotent retry

- **WHEN** publication is retried and an expected tag already points at the prepared commit
- **THEN** the tag step is treated as complete rather than moving or recreating the tag

#### Scenario: Conflicting tag

- **WHEN** an expected tag exists at another commit
- **THEN** publication stops without moving or deleting the tag

### Requirement: Publishable module validation

The release system SHALL validate that every public nested Go module is
registered, has the expected module path, and contains no local filesystem
replacement.

#### Scenario: Unregistered public module

- **WHEN** a provider or middleware `go.mod` exists without a registry entry
- **THEN** release validation fails and identifies the missing module

#### Scenario: Local replacement

- **WHEN** a registered public module contains a local `replace` directive
- **THEN** release validation fails before planning or publication

### Requirement: Prerelease lifecycle

The release system SHALL support incrementing a named prerelease channel and
promoting a prerelease version to its stable base version.

#### Scenario: Continue alpha channel

- **WHEN** the current version is `v0.2.0-alpha.1` and another release is planned in the alpha channel
- **THEN** the next version is `v0.2.0-alpha.2` unless a larger base-version bump is requested

#### Scenario: Promote alpha to stable

- **WHEN** stable promotion is requested for `v0.2.0-alpha.2`
- **THEN** the planned version is `v0.2.0`
