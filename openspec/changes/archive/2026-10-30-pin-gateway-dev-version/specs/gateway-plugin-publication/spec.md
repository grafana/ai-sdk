# Spec Delta

## MODIFIED Requirements

### Requirement: Dev-only Grafana Cloud scope
Publication SHALL target only the dev catalog, with the `grafana_cloud` scope, and SHALL produce a signed package. It SHALL NOT publish to the ops or prod catalogs, or create Git tags, GitHub releases or documentation releases. Apart from the dev version pin request, it SHALL NOT start or request a deployment of the plugin.

#### Scenario: Catalog targets
- **WHEN** a `main` push is published
- **THEN** the version SHALL exist in the dev catalog with the `grafana_cloud` scope and in no other catalog
- **AND** no Git tag, GitHub release or documentation release SHALL be created by the publication
- **AND** no ops or prod version pin SHALL change

### Requirement: Publication documentation
Maintainer documentation SHALL describe how a `main` commit becomes a dev catalog version and reaches the dev environment, including the version format, scope, gates, version pin and one-time prerequisites, and how to roll dev back to an earlier published version. It SHALL NOT name private infrastructure repositories.

#### Scenario: Maintainer looks up a commit's version
- **WHEN** a maintainer follows the documentation for a merged commit
- **THEN** they SHALL be able to tell whether and under which version it was published to the dev catalog, and whether dev runs it

#### Scenario: Maintainer rolls back dev
- **WHEN** a published version misbehaves in dev
- **THEN** the documentation SHALL describe pinning an earlier published version, and that the next `main` publication replaces that pin unless the faulty commit is reverted

## ADDED Requirements

### Requirement: Dev version pin request
After a version is published to the dev catalog, the publish workflow SHALL open an auto-merge pull request in the deployment repository that pins the dev environment to exactly that version, replacing any earlier pin pull request that has not merged. It SHALL refuse a version that does not end in `+` and the published commit SHA. Pull requests, forks and failed or skipped publications SHALL NOT request a pin.

#### Scenario: Publication requests the dev pin
- **WHEN** commit `<sha>` is published to the dev catalog as `0.1.0-alpha.1+<sha>`
- **THEN** a pull request SHALL pin the dev environment to `0.1.0-alpha.1+<sha>`

#### Scenario: Newer publication replaces an open pin
- **WHEN** a pin pull request for an earlier version has not merged and a newer version is published
- **THEN** the pin pull request SHALL be replaced by one for the newer version

#### Scenario: Failed publication requests no pin
- **WHEN** the dev publication for a commit fails or is skipped
- **THEN** no pin pull request SHALL be opened for that commit

#### Scenario: Version does not match the commit
- **WHEN** the published version is empty or does not end in `+<sha>` of the commit
- **THEN** the job SHALL fail without requesting a pin
