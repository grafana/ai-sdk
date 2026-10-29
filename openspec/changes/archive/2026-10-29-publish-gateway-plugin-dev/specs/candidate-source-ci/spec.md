# Spec Delta

## MODIFIED Requirements

### Requirement: Gateway publication is independently gated

Gateway compilation against published first-party versions SHALL NOT block plugin release or main deployment. Gateway publication SHALL run only for an eligible push after that push's required CI run shows its source, manifest-aligned requirement, boundary and plugin-validation jobs succeeded for that revision. Publication SHALL NOT run inside, delay or change the conclusion of the required CI run. Deployment SHALL follow publication.

#### Scenario: Unready Gateway artifact

- **WHEN** coordinated source checks pass but Gateway could not compile against published first-party versions
- **THEN** the source PR SHALL remain eligible to merge and plugin publication MAY proceed only if the plugin package built from same-revision source, its validation and the protocol test host suites at that push SHA pass
- **AND** SDK/provider/middleware module releases SHALL still require their own release evidence

#### Scenario: Valid artifact

- **WHEN** required source checks, boundary and requirement checks, plugin package validation and protocol test host suites all pass for the same eligible push revision
- **THEN** publication MAY proceed using that revision's source, and main deployment MAY follow successful publication

#### Scenario: Failed source, image or smoke check

- **WHEN** a required candidate-source check, build, plugin validation, requirement or boundary check fails, is skipped or is cancelled
- **THEN** Gateway publication and deployment SHALL remain blocked

#### Scenario: Publication without CI evidence

- **WHEN** publication would run for a pull request, or before the push's required CI run shows every required source and plugin-validation job succeeded for that commit
- **THEN** no publication SHALL occur

#### Scenario: Release evidence is unaffected

- **WHEN** a Gateway publication runs, waits or is superseded for a `main` push
- **THEN** that push's required CI run SHALL complete and conclude exactly as it would without publication
