# candidate-source-ci Specification

## Purpose

Validate coordinated candidate SDK and Gateway source independently of standalone artifact readiness, while keeping publication fail-closed.

## Requirements

### Requirement: Required CI integrates candidate source
Required source CI SHALL build, vet, lint, test and exercise cross-language integration against candidate SDK, providers, middleware and Gateway. It SHALL retain formatting, docs, license/isolation, parity/conformance, structural boundary, module-policy and canonical merged-pin checks. Real published internal pins MAY lag candidate source but SHALL remain downloadable and merged in canonical main; required source checks SHALL NOT compile candidate code against those pins as a prerequisite to merge.

#### Scenario: Coordinated source with older merged pins
- **WHEN** candidate root/provider/middleware/Gateway changes work together but an older merged dependency pin cannot compile that candidate independently
- **THEN** required source checks SHALL execute and validate the candidate modules without requiring a standalone build to pass
- **AND** unmerged or unverifiable pins, forbidden reverse dependencies and real source regressions SHALL still fail their required checks

#### Scenario: ProviderWire candidate differential
- **WHEN** the required ProviderWire client differential and request-mutation controls run
- **THEN** the Go capture SHALL build and run candidate Grafana client against candidate SDK root, retaining the pinned TypeScript comparator
- **AND** every copied Go-client mutant SHALL be selected by its test workspace and rejected by a semantic differential assertion, not a build/setup failure

### Requirement: Gateway publication is independently gated
Standalone selected-module and all-module commands SHALL remain available without directly or indirectly blocking an ordinary source PR. On an eligible canonical main or Gateway-tag push, image validation and publication SHALL use a fresh clean checkout, verify HEAD equals the push SHA with standard Git cleanliness checks, and build from a narrowly filtered Gateway Dockerfile-specific context. Image validation SHALL then build multiarchitecture and native Gateway images using the explicit Gateway workspace source and smoke-test the native image. Gateway standalone module compilation against published pins SHALL NOT block its container image release or main deployment. Publication SHALL depend on successful required source checks, merged-pin and one-way boundary checks, and image-validation jobs for that revision; main deployment SHALL follow successful publication. A skipped, cancelled or failed source or image-validation job SHALL NOT authorize publication or deployment.

#### Scenario: Unready Gateway artifact
- **WHEN** coordinated source checks pass but standalone Gateway cannot compile with its older merged declared dependencies
- **THEN** the source PR SHALL remain eligible to merge and image publication MAY proceed only if workspace-built images and native smoke at that push SHA pass
- **AND** standalone SDK/provider/middleware module releases SHALL still require their own validation

#### Scenario: Valid artifact
- **WHEN** required source checks, boundary and merged-pin checks, multiarchitecture image validation and native smoke all pass for the same eligible push revision
- **THEN** publication MAY proceed using that revision's workspace source, and main deployment MAY follow successful publication

#### Scenario: Failed source, image or smoke check
- **WHEN** a required candidate-source check, build, smoke, ancestry or boundary check fails, is skipped or is cancelled
- **THEN** Gateway publication and deployment SHALL remain blocked

### Requirement: Release automation remains separate
A green source PR SHALL NOT authorize an SDK release or change required-check/ruleset settings. Any required-check identity change SHALL require coordinated maintainer approval without bypassing checks. SDK release automation remains owned by #245/#21.

#### Scenario: Source PR passes
- **WHEN** a coordinated source PR passes required checks
- **THEN** no SDK tag or module publication SHALL be created by this workflow
