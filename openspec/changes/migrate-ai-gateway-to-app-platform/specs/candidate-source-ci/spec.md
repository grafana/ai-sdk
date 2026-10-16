# Spec Delta

## MODIFIED Requirements

### Requirement: Gateway publication is independently gated

Standalone selected-module and all-module commands SHALL remain available without directly or indirectly blocking ordinary source PRs. Gateway standalone compilation against published pins SHALL NOT block plugin release or main deployment. Until a Gateway plugin publication workflow exists, CI SHALL NOT publish or deploy the Gateway. Any publication workflow SHALL require successful required source, merged-pin, one-way boundary and plugin-validation jobs for that revision, and main deployment SHALL follow successful publication.

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

## ADDED Requirements

### Requirement: Gateway publication checkout and plugin validation
Plugin validation SHALL run on pull requests and eligible pushes. On eligible canonical main or Gateway-tag pushes, plugin validation SHALL use a fresh clean checkout, verify HEAD equals the push SHA with standard Git cleanliness checks, build the plugin backend with the explicit Gateway workspace, check generated manifest consistency and run plugin package validation.

#### Scenario: Gateway publication checkout and plugin validation
- **WHEN** plugin validation runs for an eligible Gateway-tag push
- **THEN** the clean push-SHA checkout SHALL build and validate the workspace-source plugin package and retain it as the validated artifact

## REMOVED Requirements

### Requirement: Gateway publication checkout and image validation
**Reason**: The Gateway container image is removed.
**Migration**: Use "Gateway publication checkout and plugin validation".
