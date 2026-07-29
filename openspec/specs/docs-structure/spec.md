## Purpose

Define the structure, ownership, and delivery conventions for the project's
narrative documentation so that content is consistently organized, GitHub-native,
and free of duplication across godoc, the root README, and the `docs/` directory.

## Requirements

### Requirement: Documentation directory taxonomy

The project SHALL provide a `docs/` directory organized into the top-level
sections `getting-started/`, `concepts/`, `guides/`, `providers/`,
`middleware/`, and `best-practices/`. The `docs/` directory SHALL contain a
`docs/README.md` index that lists and links every documentation page. The index
MAY organize links by developer outcome rather than mirroring the directory
taxonomy.

#### Scenario: Required sections exist

- **WHEN** the `docs/` directory is inspected
- **THEN** it contains the folders `getting-started/`, `concepts/`, `guides/`,
  `providers/`, `middleware/`, and `best-practices/`
- **AND** it contains a `docs/README.md` index file

#### Scenario: Index links every page

- **WHEN** a new documentation page is added under any section
- **THEN** `docs/README.md` SHALL include a link to that page under an
  appropriate developer journey or topic group

#### Scenario: Audience-to-section mapping is honored

- **WHEN** content is authored
- **THEN** onboarding/usage content lives under `getting-started/` or `guides/`,
  conceptual/mental-model content lives under `concepts/`, per-provider
  narrative lives under `providers/`, middleware overview and integration
  guidance lives under `middleware/`, and operational guidance lives under
  `best-practices/`

### Requirement: Content ownership contract

Documentation SHALL follow a single ownership rule across three surfaces:
godoc owns API reference (signatures, option lists, field-level detail); the
root `README.md` owns the landing page (pitch, install, one quick start, and a
router into `docs/`); and `docs/` owns concepts, task guides, per-provider
narrative, and best practices. `docs/` pages SHALL NOT reproduce exhaustive
option tables or symbol signatures and SHALL instead link to pkg.go.dev.

#### Scenario: docs defer reference to godoc

- **WHEN** a `docs/` page needs to reference an option, type, or function
  signature
- **THEN** it SHALL link to the corresponding pkg.go.dev symbol rather than
  reproducing the full signature or option table

#### Scenario: README is a landing page, not a reference

- **WHEN** the root `README.md` is reviewed after this change
- **THEN** it contains the pitch, install, exactly one quick start, an
  architecture summary with a link into `concepts/`, and a "where to go next"
  router into `docs/`
- **AND** it does NOT contain deep API reference (option tables, per-feature
  deep-dives) that now lives in `docs/` and godoc

#### Scenario: Reference content is relocated, not duplicated

- **WHEN** reference content is removed from the README
- **THEN** its narrative form lives in `docs/` and its reference form lives in
  godoc, with no third duplicated copy

### Requirement: GitHub-native delivery

Documentation SHALL be authored as plain GitHub-rendered markdown with no static
site generator, no frontmatter, and no sidebar configuration. Navigation SHALL
be provided by the `docs/README.md` index and by per-page `← Prev · Up · Next →`
footers. Diagrams SHALL use mermaid (GitHub-rendered) or ASCII.

#### Scenario: Pages render on GitHub without a build step

- **WHEN** any `docs/` page is viewed on GitHub
- **THEN** it renders correctly using only standard GitHub markdown features
  (no frontmatter, no site-generator-specific syntax)

#### Scenario: Pages provide navigation

- **WHEN** a reader opens any non-index `docs/` page
- **THEN** the page ends with a navigation footer linking to the previous page,
  the section/index, and the next page where applicable

### Requirement: Runnable examples are external and linked

Complete, runnable example programs SHALL live under a top-level `/examples` Go
directory as self-contained modules. The collection SHALL be curated around
recognizable application outcomes rather than providing one runnable directory
for every individual API call. Each example SHALL compile via `go build`, SHALL
provide deterministic credential-free behavioral tests, and those tests SHALL
run in blocking CI. `docs/` pages MAY include short illustrative snippets but
SHALL link to the full program in `/examples` for non-trivial end-to-end
scenarios rather than embedding the complete program.

#### Scenario: Example programs are buildable

- **WHEN** an example program is added or changed under `/examples`
- **THEN** it SHALL compile via the repository's example build task
- **AND** the example build task SHALL run in blocking CI

#### Scenario: Example behavior is tested

- **WHEN** an example module is added or changed
- **THEN** it SHALL include deterministic tests for its application-visible
  success path and important boundary failures
- **AND** the tests SHALL NOT require provider credentials or external network
  access
- **AND** the repository's example test task SHALL discover and execute the
  module in blocking CI

#### Scenario: Frontend example behavior is checked across languages

- **WHEN** an example demonstrates integration with an upstream frontend hook
- **THEN** the cross-language integration suite SHALL exercise the representative
  stream through the registered upstream frontend package version
- **AND** it SHALL assert the frontend-visible state central to the example

#### Scenario: Examples represent application outcomes

- **WHEN** the runnable example collection is reviewed
- **THEN** each top-level example SHALL correspond to a distinct application
  outcome or integration boundary
- **AND** focused API techniques that do not require a complete application
  SHALL remain in the README, guides, or godoc rather than requiring another
  runnable module

#### Scenario: Guides link to runnable code

- **WHEN** a guide demonstrates a non-trivial end-to-end scenario
- **THEN** it SHALL link to the corresponding runnable program under `/examples`
  rather than embedding the complete program inline

### Requirement: User-facing narrative is centralized in docs/

User-facing narrative documentation SHALL be centralized under `docs/` rather
than scattered across package-local READMEs. Module/package `README.md` files
that document user-facing setup, usage, or behavior SHALL have their content
folded into the appropriate `docs/` page and SHALL be removed. All `doc.go`
files SHALL remain co-located with their code as the godoc API reference and
SHALL NOT be moved. Contributor/tooling READMEs that are not user-facing (e.g.
under `test/`) MAY remain co-located.

#### Scenario: User-facing package READMEs are centralized

- **WHEN** a package has a README documenting user-facing setup or behavior
  (e.g. a provider or middleware module)
- **THEN** its content SHALL live in the relevant `docs/` page and the
  package-local README SHALL be removed

#### Scenario: doc.go files are not moved

- **WHEN** the centralization is applied
- **THEN** all `doc.go` files remain in their original locations as the godoc
  API reference

#### Scenario: docs hold the detail directly

- **WHEN** `docs/providers/grafana-cloud.md` covers auth, identity forwarding,
  or wire details
- **THEN** it SHALL contain that detail directly (linking to pkg.go.dev for the
  API reference) rather than deferring to a package-local README

### Requirement: Stub pages are consistent and intentional

Documentation pages that are not yet written to completion SHALL use a shared
stub template that includes a title, a short intro, a visible status note, a
"see also" section, and a working navigation footer, so that incomplete sections
read as intentional rather than broken.

#### Scenario: Stub page is well-formed

- **WHEN** a section is scaffolded but not yet fully written
- **THEN** its page contains a title, intro, a visible status note, a "see also"
  section, and a navigation footer
