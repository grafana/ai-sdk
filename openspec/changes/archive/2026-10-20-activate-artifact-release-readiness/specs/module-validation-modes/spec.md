## ADDED Requirements

### Requirement: Selected consumer regression evidence

Library readiness SHALL run relevant deterministic consumer regression scenarios
against the candidate library and its selected published dependency graph.
Workspace substitutions for prerequisites SHALL NOT supply release evidence.
Unrelated candidate modules SHALL NOT be required to compile.

#### Scenario: Older dependency compiles but lacks behavior

- **WHEN** a selected dependency omits runtime changes required through the candidate base
- **THEN** freshness validation fails even if compilation succeeds

#### Scenario: Selected provider consumer

- **WHEN** a provider release is validated
- **THEN** consumer scenarios use that candidate provider with published prerequisites and no local replacement

#### Scenario: Root with unready downstream source

- **WHEN** root is releasable and provider source awaits a new root version
- **THEN** root consumer validation does not import the unready provider source

### Requirement: Deterministic prerequisite source ownership

Prerequisite freshness SHALL compare explicitly owned shipped-source snapshots at
the prerequisite revision and candidate base. Ownership SHALL use the longest
registered module root, retain root-owned middleware, and include runtime assets.
Excluded development-only files SHALL be enumerated and tested.

#### Scenario: Root-owned middleware

- **WHEN** `middleware/extract_json.go` changes
- **THEN** it changes root's prerequisite snapshot
- **AND** nested `middleware/logger` files remain owned by Logger

#### Scenario: Documentation-only difference

- **WHEN** prerequisite revisions differ only in excluded documentation or tests
- **THEN** that difference does not alone require a prerequisite release

#### Scenario: Embedded runtime asset

- **WHEN** a shipped runtime asset changes
- **THEN** freshness rejects an older prerequisite even if the asset uses a documentation-like extension
