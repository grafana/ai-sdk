## 1. Release Metadata

- [x] 1.1 Add and validate the explicit public-module registry
- [x] 1.2 Add strict change-fragment parsing and generation
- [x] 1.3 Add initial per-module changelog files

## 2. Planning

- [x] 2.1 Implement strict semantic-version and prerelease calculations
- [x] 2.2 Discover current versions from root and path-prefixed Git tags
- [x] 2.3 Aggregate fragments into deterministic dependency-ordered release plans
- [x] 2.4 Add text and JSON plan output with table-driven tests

## 3. Preparation and Publication

- [x] 3.1 Implement release preparation for changelogs, root requirements, consumed fragments, and `release/plan.json`
- [x] 3.2 Implement publishability checks for module paths, registry coverage, local replacements, and prepared-plan consistency
- [x] 3.3 Implement dry-run and explicitly confirmed dependency-ordered tag and GitHub Release publication
- [x] 3.4 Add retry, conflicting-tag, and partial-publication tests using fake command execution or temporary repositories

## 4. Agent and Repository Integration

- [x] 4.1 Add a single `mise run release -- <command>` entrypoint and focused CI validation
- [x] 4.2 Create the repository-local `release-ai-sdk` skill and generated agent metadata
- [x] 4.3 Add a manually dispatched, explicitly confirmed publication workflow

## 5. Verification

- [x] 5.1 Run release command unit tests and synthetic end-to-end planning/preparation tests
- [x] 5.2 Run skill validation and exercise the skill against representative release requests
- [x] 5.3 Run formatting, vet, focused repository tests, and OpenSpec validation

## 6. Selective Release Cadence

- [x] 6.1 Add repeatable module selection to deterministic planning
- [x] 6.2 Preserve and validate deferred intent when preparation partially consumes a fragment
- [x] 6.3 Add selective planning and preparation tests, including shared fragments
- [x] 6.4 Add the co-located release runbook and update contributor and agent guidance
