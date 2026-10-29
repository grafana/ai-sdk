# Spec Delta

## MODIFIED Requirements

### Requirement: Gateway publication is independently gated

Gateway compilation against published first-party versions SHALL NOT block plugin release or main deployment. Gateway publication SHALL run only within the required CI run of an eligible push, after its source, manifest-aligned requirement, boundary and plugin-validation jobs pass for that revision. Deployment SHALL follow publication.

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

#### Scenario: Publication outside the gated run

- **WHEN** a workflow other than the required CI run of the push, or a run for a pull request, would publish the Gateway
- **THEN** no publication SHALL occur
