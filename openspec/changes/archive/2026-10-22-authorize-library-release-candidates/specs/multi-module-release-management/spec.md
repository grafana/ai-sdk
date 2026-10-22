# Spec Delta

## ADDED Requirements

### Requirement: Frozen merged-library candidate inventory

Authorization SHALL operate on a complete, frozen inventory of pending merged library releases. Each record SHALL identify the canonical repository, component, version, tag, PR, release head, merge revision, merge parent, tree and generated metadata. Discovery failures, ambiguous candidates and unsupported components SHALL NOT yield authorization. New arrivals SHALL require a later inventory.

#### Scenario: Candidate arrives during validation

- **WHEN** another release PR merges after the inventory is frozen
- **THEN** it is absent from the current authorization results and cannot enter the approved set

#### Scenario: Partial discovery

- **WHEN** candidate discovery fails before all requested pages are read
- **THEN** the run fails without emitting an approved set from the partial inventory

#### Scenario: Conflicting candidates

- **WHEN** multiple pending candidates claim the same component or tag
- **THEN** those candidates are blocked with an explicit ambiguity reason

### Requirement: Authentic merged-library identity and tree

Authorization SHALL verify immutable release App and repository identity, a signed release head, canonical main ancestry, one generated component change and consistent version/notes/tag metadata. The supported squash revision SHALL have one parent and the same complete tree as the successfully validated PR candidate; its parent SHALL match that candidate's validated base. Missing proof SHALL block authorization.

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
- **AND** standalone and consumer checks use the selected merged library with published prerequisites

### Requirement: Independent library authorization decisions

Authorization SHALL report each unambiguous library candidate separately. A selected library's actual prerequisite failure SHALL block that candidate; unrelated downstream standalone failures SHALL NOT block a ready candidate. Existing candidate-source checks SHALL remain required. Gateway SHALL receive no library authorization.

#### Scenario: Root ready and provider blocked

- **WHEN** root passes its merged-source and selected-library checks while a provider requires an unreleased root prerequisite
- **THEN** root is approved in the report and the provider is blocked with its own reason
- **AND** root validation does not invoke the provider's standalone checks

#### Scenario: Gateway is selected

- **WHEN** a Gateway candidate is discovered or explicitly selected
- **THEN** it is identified as unsupported by this library authorization mode

### Requirement: Read-only authorization boundary

The authorization mode SHALL perform no GitHub mutations, tag writes, release publication or activation changes. Its report SHALL identify the immutable candidates, evidence and tooling revision, and state that it grants no standing publication permission. Changed metadata, evidence or tag state SHALL invalidate a previous decision; future publication SHALL revalidate rather than trust an editable report.

#### Scenario: Tag already exists

- **WHEN** a candidate tag or corresponding release already exists, or its state cannot be verified
- **THEN** this slice blocks authorization and reports that publication recovery is outside its scope

#### Scenario: Evidence changes during validation

- **WHEN** candidate metadata, relevant run attempt or tag state changes before the final recheck
- **THEN** the affected candidate is not approved

#### Scenario: Attempted publication

- **WHEN** the authorization driver attempts a remote mutation or publication API
- **THEN** the transport rejects it and no tag, release, label or PR mutation occurs
