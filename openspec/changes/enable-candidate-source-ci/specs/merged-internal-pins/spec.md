## MODIFIED Requirements

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
