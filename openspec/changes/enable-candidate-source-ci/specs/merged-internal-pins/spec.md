## MODIFIED Requirements

### Requirement: Reviewed migration and guard activation
Existing unmerged published internal pins SHALL be inventoried and replaced only with already-merged revisions that retain the needed behavior. The migration SHALL record old and new full-commit provenance and standalone public-proxy, readonly, workspace-off validation. The merged-pin guard SHALL remain a required source check after source CI activates candidate integration; standalone compilation SHALL remain an independently callable diagnostic and a blocking artifact-publication check, not a required source PR check. A merged pin MAY lag candidate source without relaxing canonical ancestry, declared/selected version verification or the no-local-replacement rule.

#### Scenario: No suitable merged revision
- **WHEN** no already-merged revision provides behavior needed by an existing branch-only pin
- **THEN** migration SHALL stop and escalate instead of pinning another branch-only revision or silently choosing the latest commit

#### Scenario: Migration passes
- **WHEN** reviewed replacements pass standalone validation and the merged-pin check
- **THEN** required source CI SHALL enforce merged ancestry and structural boundary checks independently of standalone compilation

#### Scenario: Older merged dependency
- **WHEN** a candidate source PR depends on local coordinated changes while its older published internal pins resolve to canonical merged commits
- **THEN** the merged-pin guard SHALL pass even if standalone compilation fails

#### Scenario: Unmerged or unverifiable dependency
- **WHEN** a declared or selected internal pin is branch-only, unknown or cannot be verified against canonical main
- **THEN** required source CI SHALL fail closed despite successful candidate-source integration
