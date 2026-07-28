## MODIFIED Requirements

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
