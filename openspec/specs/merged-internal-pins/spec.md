# merged-internal-pins Specification

## Purpose

Ensure published internal Go module versions resolve to commits already merged into canonical `grafana/ai-sdk` `main` without mistaking downloadability or local-only placeholders for merge evidence.

## Requirements

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
