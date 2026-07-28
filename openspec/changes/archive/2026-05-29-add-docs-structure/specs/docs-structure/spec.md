## ADDED Requirements

### Requirement: Documentation directory taxonomy

The project SHALL provide a `docs/` directory organized into a fixed set of
top-level sections: `getting-started/`, `concepts/`, `guides/`, `providers/`,
and `best-practices/`. The `docs/` directory SHALL contain a `docs/README.md`
index that lists and links every documentation page, grouped by section.

#### Scenario: Required sections exist

- **WHEN** the `docs/` directory is inspected
- **THEN** it contains the folders `getting-started/`, `concepts/`, `guides/`,
  `providers/`, and `best-practices/`
- **AND** it contains a `docs/README.md` index file

#### Scenario: Index links every page

- **WHEN** a new documentation page is added under any section
- **THEN** `docs/README.md` SHALL include a link to that page under the
  appropriate section group

#### Scenario: Audience-to-section mapping is honored

- **WHEN** content is authored
- **THEN** onboarding/usage content lives under `getting-started/` or `guides/`,
  conceptual/mental-model content lives under `concepts/`, operational
  guidance lives under `best-practices/`, and per-provider narrative lives under
  `providers/`

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
directory and SHALL be `go build`-verifiable. `docs/` pages MAY include short
illustrative snippets but SHALL link to the full program in `/examples` for
non-trivial examples rather than embedding the complete program.

#### Scenario: Example programs are buildable

- **WHEN** an example program is added under `/examples`
- **THEN** it SHALL compile via `go build`

#### Scenario: Guides link to runnable code

- **WHEN** a guide demonstrates a non-trivial end-to-end scenario
- **THEN** it SHALL link to the corresponding runnable program under `/examples`
  rather than embedding the complete program inline

### Requirement: Package-local docs remain co-located

Existing package-local READMEs and all `doc.go` files SHALL remain co-located
with their code per Go convention and SHALL NOT be moved into `docs/`. Pages
under `docs/providers/` SHALL link to the relevant package-local README rather
than copying its content.

#### Scenario: Package READMEs are not moved

- **WHEN** the change is applied
- **THEN** `providers/grafana/README.md`, `test/README.md`, and all `doc.go`
  files remain in their original locations

#### Scenario: Provider docs link to package README

- **WHEN** `docs/providers/grafana-cloud.md` covers auth or wire details that
  exist in `providers/grafana/README.md`
- **THEN** it SHALL link to that package README rather than duplicating the
  detail

### Requirement: Stub pages are consistent and intentional

Documentation pages that are not yet written to completion SHALL use a shared
stub template that includes a title, a short intro, a visible status note, a
"see also" section, and a working navigation footer, so that incomplete sections
read as intentional rather than broken.

#### Scenario: Stub page is well-formed

- **WHEN** a section is scaffolded but not yet fully written
- **THEN** its page contains a title, intro, a visible status note, a "see also"
  section, and a navigation footer
