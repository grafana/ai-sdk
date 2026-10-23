# Spec Delta

## ADDED Requirements

### Requirement: Bounded library publication

Publication SHALL use only freshly authorized candidates from one frozen inventory. Each mutation SHALL match the selected candidate's immutable tag, revision and generated release metadata. Publication SHALL NOT rescan for additional candidates or accept a saved report as authorization. Unrelated blocked candidates SHALL retain independent outcomes.

#### Scenario: Late candidate arrival

- **WHEN** another candidate arrives after discovery
- **THEN** publication does not include it in the current batch

#### Scenario: Evidence changes before a write

- **WHEN** PR metadata, required evidence, ancestry or expected tag state changes
- **THEN** the candidate's next mutation is blocked

#### Scenario: Unrelated provider is blocked

- **WHEN** a provider cannot validate its prerequisites but root validates
- **THEN** only root may proceed to its bounded publication operations

### Requirement: Immutable library publication recovery

Publication retries SHALL revalidate the original candidate and inspect canonical tag and release state. Matching partial publication MAY finish without modifying existing tags or releases. Conflicting or unverifiable state SHALL block writes. Completed releases SHALL not be recreated; lifecycle completion SHALL remain discoverable after partial failure.

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

### Requirement: Serialized inactive publication integration

The publication entrypoint SHALL remain disabled until separate activation approval. The integration SHALL serialize candidate writes, reject overlapping invocations and preserve the canonical workflow's non-cancelling concurrency boundary. The read-only authorization command SHALL retain its no-mutation behavior and existing-tag rejection.

#### Scenario: Invocation before activation

- **WHEN** the publication command is invoked before activation
- **THEN** it refuses before credentials or network operations

#### Scenario: Overlapping invocation

- **WHEN** another publication invocation is already executing in the process
- **THEN** the second invocation fails without writes

#### Scenario: Read-only authorization sees an existing tag

- **WHEN** read-only authorization sees any existing candidate tag
- **THEN** it remains blocked regardless of publisher recovery support
