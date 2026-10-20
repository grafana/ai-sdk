# candidate-source-ci Specification

## Purpose

Validate coordinated candidate SDK and Gateway source independently of standalone artifact readiness, while keeping publication fail-closed.

## Requirements

### Requirement: Required CI integrates candidate source

Required source CI SHALL build, vet, lint, test and exercise cross-language integration against candidate SDK, providers, middleware and Gateway. It SHALL retain formatting, docs, license/isolation, parity/conformance, structural boundary, module-policy and canonical merged-pin checks. Real published internal pins MAY lag candidate source but SHALL remain downloadable and merged in canonical main; compiling candidate code against those pins SHALL NOT be a merge prerequisite.

#### Scenario: Coordinated source with older merged pins
- **WHEN** candidate root/provider/middleware/Gateway changes work together but an older merged dependency pin cannot compile that candidate independently
- **THEN** required source checks SHALL execute and validate the candidate modules without requiring a standalone build to pass
- **AND** unmerged or unverifiable pins, forbidden reverse dependencies and real source regressions SHALL still fail their required checks

#### Scenario: ProviderWire candidate differential
- **WHEN** the required ProviderWire client differential and request-mutation controls run
- **THEN** the Go capture SHALL build and run candidate Grafana client against candidate SDK root, retaining the pinned TypeScript comparator
- **AND** every copied Go-client mutant SHALL be selected by its test workspace and rejected by a semantic differential assertion, not a build/setup failure

### Requirement: Gateway publication is independently gated

Standalone selected-module and all-module commands SHALL remain available without blocking ordinary source PRs. Gateway standalone compilation against published pins SHALL NOT block plugin release or main deployment. Until a Gateway plugin publication workflow exists, CI SHALL NOT publish or deploy the Gateway. Any publication workflow SHALL require the source, merged-pin, boundary and plugin-validation jobs to pass for that revision, and deployment SHALL follow publication.

#### Scenario: Unready Gateway artifact
- **WHEN** coordinated source checks pass but standalone Gateway cannot compile with its older merged declared dependencies
- **THEN** the source PR SHALL remain eligible to merge and plugin publication MAY proceed only if the workspace-built plugin package, its validation and the protocol test host suites at that push SHA pass
- **AND** standalone SDK/provider/middleware module releases SHALL still require their own validation

#### Scenario: Valid artifact
- **WHEN** required source checks, boundary and merged-pin checks, plugin package validation and protocol test host suites all pass for the same eligible push revision
- **THEN** publication MAY proceed using that revision's workspace source, and main deployment MAY follow successful publication

#### Scenario: Failed source, image or smoke check
- **WHEN** a required candidate-source check, build, plugin validation, ancestry or boundary check fails, is skipped or is cancelled
- **THEN** Gateway publication and deployment SHALL remain blocked

### Requirement: Gateway publication fails closed on job state

A skipped, cancelled or failed required source or plugin-validation job SHALL NOT authorize publication or deployment.

#### Scenario: Gateway publication fails closed on job state
- **WHEN** a plugin-validation job is cancelled
- **THEN** Gateway publication and main deployment SHALL remain unauthorized

### Requirement: Release automation remains separate
A green source PR SHALL NOT authorize an SDK release or change required-check/ruleset settings. Any required-check identity change SHALL require coordinated maintainer approval without bypassing checks. SDK release automation remains owned by #245/#21.

#### Scenario: Source PR passes
- **WHEN** a coordinated source PR passes required checks
- **THEN** no SDK tag or module publication SHALL be created by this workflow

### Requirement: Gateway publication checkout and plugin validation
Plugin validation SHALL run on pull requests and eligible pushes. On eligible canonical main or Gateway-tag pushes, plugin validation SHALL use a fresh clean checkout, verify HEAD equals the push SHA with standard Git cleanliness checks, build the plugin backend with the explicit Gateway workspace, check generated manifest consistency and run plugin package validation.

#### Scenario: Gateway publication checkout and plugin validation
- **WHEN** plugin validation runs for an eligible Gateway-tag push
- **THEN** the clean push-SHA checkout SHALL build and validate the workspace-source plugin package and retain it as the validated artifact
