## MODIFIED Requirements

### Requirement: Gateway publication is independently gated
Standalone selected-module and all-module commands SHALL remain available without directly or indirectly blocking an ordinary source PR. On an eligible canonical main or Gateway-tag push, image validation and publication SHALL verify HEAD equals the push SHA and Docker-relevant inputs match that commit, rejecting admitted untracked inputs before building. Image validation SHALL then build multiarchitecture and native Gateway images using the explicit Gateway workspace source and smoke-test the native image. Gateway standalone module compilation against published pins SHALL NOT block its container image release or main deployment. Publication SHALL depend on successful required source checks, merged-pin and one-way boundary checks, and image-validation jobs for that revision; main deployment SHALL follow successful publication. A skipped, cancelled or failed source or image-validation job SHALL NOT authorize publication or deployment.

#### Scenario: Gateway standalone pins lag candidate source
- **WHEN** coordinated source checks pass but standalone Gateway cannot compile with its older merged declared dependencies
- **THEN** image validation and publication MAY proceed only if workspace-built images and native smoke at that push SHA pass
- **AND** standalone SDK/provider/middleware module releases SHALL still require their own validation

#### Scenario: Valid container artifact
- **WHEN** required source checks, boundary and merged-pin checks, multiarchitecture image validation and native smoke all pass for the same eligible push revision
- **THEN** publication MAY proceed using that revision's workspace source, and main deployment MAY follow successful publication

#### Scenario: Failed source, image or smoke check
- **WHEN** a required candidate-source check, build, smoke, ancestry or boundary check fails, is skipped or is cancelled
- **THEN** Gateway publication and deployment SHALL remain blocked
