## ADDED Requirements

### Requirement: Manifest-aligned first-party requirements

A required source check SHALL verify that every first-party requirement in every
tracked `go.mod`, including Gateway, example and test modules, equals the
version manifest entry of the required module, or that module's registered
initial version if it has no manifest entry. Pseudo-versions, placeholders and
other versions SHALL fail. The check SHALL identify modules by tracked roots and
declared paths and SHALL fail for an unregistered first-party module.

#### Scenario: Requirement matches the manifest

- **WHEN** a module requires root at the version recorded for root in the manifest
- **THEN** the check passes

#### Scenario: Pseudo-version or stale requirement

- **WHEN** a module requires a first-party module at a pseudo-version, a placeholder such as `v0.0.0`, or a version other than its manifest entry
- **THEN** the check fails and identifies the module, requirement and expected version

#### Scenario: Never-released module

- **WHEN** a module requires a first-party module that has no manifest entry
- **THEN** the requirement must equal that module's registered initial version

#### Scenario: Unknown first-party module

- **WHEN** a module requires a first-party module path that is not registered
- **THEN** the check fails rather than skipping it

## REMOVED Requirements

### Requirement: Canonical merged-only published internal pins

**Reason**: First-party requirements always equal manifest versions, so merged-commit ancestry of arbitrary pins no longer needs checking.

**Migration**: See "Manifest-aligned first-party requirements".

### Requirement: Merged-pin verification fails closed

**Reason**: The ancestry check it guards is removed.

**Migration**: See "Manifest-aligned first-party requirements".

### Requirement: Local-only placeholders are not published pins

**Reason**: Example and test modules follow the same manifest rule and local replacements as published modules.

**Migration**: See "Manifest-aligned first-party requirements".

### Requirement: Reviewed migration and guard activation

**Reason**: The migration to merged pins is superseded by manifest-aligned requirements with local replacements.

**Migration**: Existing pseudo-version requirements are set to manifest versions in this change.

### Requirement: Merged pins and standalone readiness are independent

**Reason**: Requirements no longer lag candidate source in builds, and committed local replacements are now required.

**Migration**: See "Same-revision first-party source through local replacements" in `module-validation-modes`.
