# Deferred activation design

These original draft deltas are retained as planning input for #245. They are
not synced by this library slice and do not establish completed implementation.
The unchecked tasks in tasks.md remain open. Future changes must reassess these
drafts against their then-current source and specs.

## Original draft: module-validation-modes

## MODIFIED Requirements

### Requirement: Standalone validation publication boundary

Selected SDK/provider/middleware standalone validation SHALL precede Go-module
publication. Gateway standalone results SHALL NOT block plugin publication or
deployment; all-module results SHALL NOT block ordinary source PRs. Gateway's
supported distribution SHALL be its plugin package even when Go tooling discovers
its application tags.

#### Scenario: Standalone validation publication boundary

- **WHEN** Go tooling discovers a Gateway tag but standalone compilation fails
- **THEN** Gateway plugin validation remains independent of that failure
- **AND** no library publication bypasses its own standalone check

## ADDED Requirements

### Requirement: Selected consumer regression evidence

Library readiness SHALL run relevant deterministic consumer regression scenarios
against the candidate library and its selected published dependency graph.
Workspace substitutions for prerequisites SHALL NOT supply release evidence.
Unrelated candidate modules SHALL NOT be required to compile.

#### Scenario: Older dependency compiles but lacks behavior

- **WHEN** a selected dependency omits runtime changes required through the candidate base
- **THEN** freshness validation fails even if compilation succeeds

#### Scenario: Selected provider consumer

- **WHEN** a provider release is validated
- **THEN** consumer scenarios use that candidate provider with published prerequisites and no local replacement

#### Scenario: Root with unready downstream source

- **WHEN** root is releasable and provider source awaits a new root version
- **THEN** root consumer validation does not import the unready provider source

### Requirement: Deterministic prerequisite source ownership

Prerequisite freshness SHALL compare explicitly owned shipped-source snapshots at
the prerequisite revision and candidate base. Ownership SHALL use the longest
registered module root, retain root-owned middleware, and include runtime assets.
Excluded development-only files SHALL be enumerated and tested.

#### Scenario: Root-owned middleware

- **WHEN** `middleware/extract_json.go` changes
- **THEN** it changes root's prerequisite snapshot
- **AND** nested `middleware/logger` files remain owned by Logger

#### Scenario: Documentation-only difference

- **WHEN** prerequisite revisions differ only in excluded documentation or tests
- **THEN** that difference does not alone require a prerequisite release

#### Scenario: Embedded runtime asset

- **WHEN** a shipped runtime asset changes
- **THEN** freshness rejects an older prerequisite even if the asset uses a documentation-like extension

## Original draft: multi-module-release-management

## MODIFIED Requirements

### Requirement: Pull-request-title-derived release intent

The release system SHALL derive library and application releases from Conventional
Commit subjects produced by squash-merging pull request titles. Library attribution
SHALL follow module ownership. Relevant linked workspace commits SHALL also
contribute to Gateway intent without synchronizing component versions.

#### Scenario: Fix commit in one module

- **WHEN** a `fix: ...` pull request touches only `providers/openai`
- **THEN** OpenAI receives patch intent and Gateway also receives intent if that provider is linked into the application
- **AND** unrelated libraries receive no intent

#### Scenario: Cross-module change

- **WHEN** a squash-merged pull request touches root and a provider
- **THEN** both libraries receive intent and a linked Gateway commit is counted once

#### Scenario: Non-releasing change

- **WHEN** a commit uses `chore`, `ci`, or `test` without a release override
- **THEN** linked attribution does not manufacture release intent

#### Scenario: Unparsable pull request title

- **WHEN** a pull request title is not a Conventional Commit
- **THEN** a required check fails before it reaches release history

#### Scenario: Gateway released more recently than its dependency

- **WHEN** a linked library has unreleased commits already included in a Gateway release
- **THEN** those commits are not replayed into another Gateway release

### Requirement: Gateway application release

Gateway SHALL remain independently versioned with changelogs, release PRs and
`ai-gateway/vX.Y.Z` tags. Main and versioned plugin packages SHALL use the workspace
at the selected revision. Gateway SHALL NOT wait for library tags, repins or
standalone compilation. Linked workspace changes SHALL contribute release intent
through release-please without a competing version calculator.

#### Scenario: Linked SDK change changes the Gateway image

- **WHEN** a relevant SDK, provider or middleware change affects workspace-built Gateway behavior
- **THEN** Gateway receives plugin application release intent without an ai-gateway path change

#### Scenario: Application release validation

- **WHEN** a Gateway release candidate is prepared
- **THEN** application tests, frontend checks, linux/amd64 and linux/arm64 plugin builds, package/license validation, Grafana end-to-end tests and plugin test host suites precede publication
- **AND** standalone library evidence cannot substitute for those results

#### Scenario: Package version edit

- **WHEN** a Gateway release PR updates its package version
- **THEN** the package version matches its release manifest and intended application tag
- **AND** changes to other package fields are rejected

### Requirement: Fail-closed activation

Release automation SHALL remain disabled until artifact readiness, current-candidate
authorization, App prerequisites and Gateway attribution are reviewed. Maintainers
SHALL approve required-check and up-to-date-base protection before activation.
Activation SHALL account for pending merged release PRs. Published tags SHALL
remain immutable.

#### Scenario: Incomplete Gateway integration

- **WHEN** plugin validation or linked attribution is incomplete
- **THEN** Gateway readiness fails and release preparation and automatic tag creation remain disabled

#### Scenario: Stale successful validation

- **WHEN** main advances after a release PR passes readiness
- **THEN** the previous success cannot authorize merging a changed candidate

## ADDED Requirements

### Requirement: Exact-candidate publication authorization

Before creating a library or application tag, automation SHALL verify the selected
merged release PR's identity, generated changes, version, merge revision and
artifact-specific readiness. The tag SHALL select that revision, never a moving
branch tip. Mutable PR labels or descriptions SHALL NOT independently authorize
publication.

#### Scenario: Main advances before publication

- **WHEN** newer source merges before the release workflow runs
- **THEN** the validated release PR revision is tagged and newer source is excluded

#### Scenario: Edited release metadata

- **WHEN** release-please's proposed version differs from the merged manifest
- **THEN** no tag is created for that candidate

#### Scenario: Multiple pending candidates

- **WHEN** a ready root release and an unready downstream release are pending
- **THEN** only the independently validated root candidate can publish

### Requirement: Immutable retry and serialized publication

Release automation SHALL serialize publication, validate existing tag targets,
and retry failures using the same candidate identity. It SHALL reject conflicting
tags and SHALL NOT move or delete published tags during recovery.

#### Scenario: Partial publication

- **WHEN** a tag exists at the approved revision but later release processing failed
- **THEN** recovery preserves the tag and completes only missing release processing

#### Scenario: Conflicting tag

- **WHEN** an existing version tag targets another revision
- **THEN** automation fails without modifying that tag

### Requirement: Prompt library prerequisite progression

Renovate SHALL propose separate source PRs for available first-party tagged library
prerequisites, including alpha releases, without the generic dependency delay.
Dependency-only updates SHALL produce release intent and preserve signed-commit
requirements. Gateway housekeeping SHALL NOT block library or application releases.

#### Scenario: Root to provider to Bedrock

- **WHEN** root publishes, then OpenAI repins and publishes
- **THEN** Renovate can advance OpenAI and subsequently Bedrock without requiring all downstream libraries to pass standalone first

#### Scenario: First nested release

- **WHEN** a never-released library has merged pseudo-version prerequisites
- **THEN** it waits for already-available tagged prerequisites in separate source PRs rather than referencing a future tag in its release PR
