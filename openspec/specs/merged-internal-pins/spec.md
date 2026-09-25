# merged-internal-pins Specification

## Purpose

Ensure published internal Go module versions resolve to commits already merged into canonical `grafana/ai-sdk` `main` without mistaking downloadability or local-only placeholders for merge evidence.

## Requirements

### Requirement: Canonical merged-only published internal pins

The repository SHALL provide an independently callable check for every real internal dependency version declared by a published Go module and selected by its dependency graph. The check SHALL identify published modules by their tracked module roots and declared module paths, resolve tags and pseudo-versions to full Git commits, and verify that those commits are ancestors of an explicitly fetched `refs/heads/main` from the canonical `grafana/ai-sdk` repository. It SHALL run without workspace substitutions or manifest mutations and SHALL fail closed on resolution, fetch, history, or verification errors. Proxy availability alone SHALL NOT establish ancestry.

#### Scenario: Merged pseudo-version
- **WHEN** a published module directly requires or selects an internal pseudo-version resolving to a commit already in canonical `main`
- **THEN** the merged-pin check SHALL accept it even if no release tag exists

#### Scenario: Merged tag
- **WHEN** a published module directly requires or selects an internal tag resolving to a commit already in canonical `main`
- **THEN** the merged-pin check SHALL accept it

#### Scenario: Branch-only commit or tag
- **WHEN** a published module directly requires or selects an internal commit or tag whose commit is not an ancestor of canonical `main`
- **THEN** the merged-pin check SHALL fail and identify the module and version

#### Scenario: Synthetic merge ref, fork, or shallow checkout
- **WHEN** a revision appears merged relative only to a fork's `main`, PR HEAD, or a synthetic merge ref, or local checkout history is shallow
- **THEN** the check SHALL verify against independently fetched canonical `main` history or fail closed; it SHALL NOT accept that revision based on local refs

#### Scenario: Unverifiable version
- **WHEN** a required or selected internal revision, its commit, or the canonical anchor cannot be resolved or verified
- **THEN** the check SHALL fail rather than infer merge status from the public proxy or the version string

#### Scenario: Unknown internal module
- **WHEN** a published module requires or selects an internal module path not registered among the tracked published module roots
- **THEN** the check SHALL fail rather than skip that dependency

### Requirement: Local-only placeholders are not published pins

The merged-pin check SHALL exclude deliberate example and test modules with local-only replacements from the published-module inventory, while preserving the separate rule that published modules have no committed local replacements. It SHALL include selected transitive internal dependencies of published modules even when those dependencies are not directly required.

#### Scenario: Example placeholder
- **WHEN** an example or test module declares a `v0.0.0` or other local-only placeholder replaced by repository source
- **THEN** the checker SHALL NOT require that placeholder to be downloadable or merged
- **AND** standalone published-module checks SHALL continue to reject local replacements in published modules

#### Scenario: Transitive selection
- **WHEN** a published module's selected dependency graph includes an internal module not directly declared in that module's `require` block
- **THEN** its selected version SHALL be checked against canonical `main`

### Requirement: Reviewed migration and guard activation

Existing unmerged published internal pins SHALL be inventoried and replaced only with already-merged revisions retaining needed behavior. That migration SHALL record old and new full-commit provenance and standalone public-proxy, readonly, workspace-off validation. The merged-pin guard SHALL remain a required source check for both declared and selected internal versions against canonical main, failing closed when a version or ancestry is unverifiable. A real, downloadable merged pin MAY lag candidate source; successful standalone compilation SHALL NOT be a source-PR prerequisite. Published module manifests SHALL NOT contain committed local replacements. Standalone validation remains callable separately for independently consumable artifacts.

#### Scenario: Older merged dependency
- **WHEN** coordinated candidate changes use real older internal pins merged into canonical main
- **THEN** the required ancestry guard SHALL pass even if standalone compilation fails

#### Scenario: Unmerged or unverifiable dependency
- **WHEN** a declared or selected internal pin is branch-only, unknown or cannot be verified against canonical main
- **THEN** required source CI SHALL fail despite successful candidate-source integration

#### Scenario: No suitable merged revision
- **WHEN** no already-merged revision provides behavior needed by an existing branch-only pin
- **THEN** migration SHALL stop and escalate instead of choosing another branch-only revision

#### Scenario: Migration passes
- **WHEN** reviewed replacements pass standalone validation and the merged-pin check
- **THEN** required source CI SHALL enforce merged ancestry independently of subsequent standalone compilation
