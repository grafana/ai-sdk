# Spec Delta

## ADDED Requirements

### Requirement: Narrative-only pull requests retain required status checks

Candidate CI MAY omit source work only for a pull request confidently classified as changing only allowlisted narrative files in regular non-executable form. Documentation lint, Gateway documentation-link validation and required status reports SHALL remain. Empty, mixed, uncertain or unclassified changes and non-pull-request events SHALL take the source path.

#### Scenario: Known Gateway prose edits

- **WHEN** a pull request changes only allowlisted documentation, including content edits to the Gateway README or changes to allowlisted Gateway guides
- **THEN** source jobs SHALL report the narrative selection without compiling or running source tests, and documentation lint and Gateway documentation-link validation SHALL run

#### Scenario: Missing linked Gateway documentation

- **WHEN** a narrative pull request removes a repository document or heading linked by the Gateway overview or plugin README
- **THEN** the documentation validation SHALL fail without requiring a plugin build

#### Scenario: Packaging input or unknown change

- **WHEN** a pull request removes the fallback Gateway README, changes a file mode or changes an input outside the narrative allowlist, or the classifier cannot establish the complete diff
- **THEN** CI SHALL run source validation

### Requirement: Candidate Gateway command contracts have one required execution owner

Source CI SHALL run the existing official OpenAI Go and TypeScript client contract scenarios once against current candidate Gateway and host executables built with the Go race detector. This ownership SHALL preserve the remaining short, full and race Gateway coverage, and SHALL NOT replace ProviderWire or other integration and plugin validation.

#### Scenario: Same candidate binaries across client suites

- **WHEN** the command-contract owner runs
- **THEN** both suites SHALL run using a single build of each executable from the candidate's local module graph, with separate test processes and fixtures
- **AND** a failed contract assertion, reported race or missing execution SHALL prevent required source success and Gateway publication

### Requirement: Tag artifact validation bypasses CI caches

Gateway tag validation SHALL neither restore nor save CI caches in the Build & Test workflow. Non-tag cache hits, misses or partial caches SHALL NOT change source-check selection, module or lockfile verification, or artifact provenance.

#### Scenario: Eligible Gateway tag push

- **WHEN** Build & Test validates a Gateway tag
- **THEN** source and artifact validation SHALL run without CI cache exchange
- **AND** only same-revision artifact evidence SHALL be eligible for publication

#### Scenario: Empty or partial dependency caches

- **WHEN** non-tag CI has an empty or partial Go module cache or pnpm download store
- **THEN** the same checks SHALL run with ordinary checked dependency resolution and frozen pnpm installs
