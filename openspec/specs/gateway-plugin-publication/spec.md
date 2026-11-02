# gateway-plugin-publication Specification

## Purpose

Publish validated Gateway app plugin packages to a plugin catalog, so Grafana Cloud environments can install a traceable version of the plugin built from a known commit.

## Requirements

### Requirement: Dev catalog publication on main pushes
Every push to `main` of the canonical repository SHALL publish the Gateway plugin package built from that commit to the dev plugin catalog, unless a newer `main` push supersedes it before its publication starts. Pull requests, tag pushes, pushes to other branches and forks SHALL NOT publish.

#### Scenario: Main push publishes
- **WHEN** a commit is pushed to `main` of the canonical repository and its required checks pass
- **THEN** the plugin package built from that commit SHALL be published to the dev catalog

#### Scenario: Pull request does not publish
- **WHEN** CI runs for a pull request, including one from the canonical repository
- **THEN** no plugin version SHALL be published to any catalog

#### Scenario: Fork does not publish
- **WHEN** a push to `main` runs in a fork of the repository
- **THEN** no plugin version SHALL be published to any catalog

### Requirement: Traceable publication version
A published version SHALL be the Gateway `package.json` version followed by `+` and the full commit SHA, so each `main` commit gets a distinct version. The package metadata SHALL report that version, and the package's source revision SHALL equal the same commit.

#### Scenario: Version identifies the commit
- **WHEN** commit `<sha>` is published while `package.json` declares `0.1.0-alpha.1`
- **THEN** the catalog version and the packaged plugin metadata SHALL be `0.1.0-alpha.1+<sha>`
- **AND** the packaged `SOURCE_REVISION` SHALL be `<sha>`

### Requirement: Dev-only Grafana Cloud scope
Publication SHALL target only the dev catalog, with the `grafana_cloud` scope, and SHALL produce a signed package. It SHALL NOT publish to the ops or prod catalogs, create Git tags, GitHub releases or documentation releases, or start a deployment of the plugin.

#### Scenario: Catalog targets
- **WHEN** a `main` push is published
- **THEN** the version SHALL exist in the dev catalog with the `grafana_cloud` scope and in no other catalog
- **AND** no Git tag, GitHub release, documentation release or deployment SHALL be created by the publication

### Requirement: Shared workflow validation before publication
Publication SHALL run Grafana's shared plugin workflow, pinned to a release tag. Its frontend and backend lint, tests and build, its plugin validator and its secret scan SHALL all pass before the package is published.

#### Scenario: Shared validation fails
- **WHEN** the shared workflow's lint, tests, build, plugin validator or secret scan fails for a commit
- **THEN** no version SHALL be published for that commit

### Requirement: Ordered publication
Publications SHALL run one at a time, in push order. A newer `main` push SHALL supersede an older one that is still waiting to publish, and SHALL NOT cancel a publication that has already started.

#### Scenario: Pushes in quick succession
- **WHEN** three commits are pushed to `main` while the first commit's publication is running
- **THEN** the first publication SHALL complete
- **AND** the third commit SHALL be published after it, and the second SHALL NOT be published

### Requirement: Publication documentation
Maintainer documentation SHALL describe how a `main` commit becomes a dev catalog version, including the version format, scope, gates and one-time prerequisites. It SHALL state that publication does not deploy or run the plugin, and SHALL NOT name private infrastructure repositories.

#### Scenario: Maintainer looks up a commit's version
- **WHEN** a maintainer follows the documentation for a merged commit
- **THEN** they SHALL be able to tell whether and under which version it was published to the dev catalog
