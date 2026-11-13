# Spec Delta

## MODIFIED Requirements

### Requirement: Dev-only Grafana Cloud scope
Publication on a `main` push SHALL target only the dev catalog, with the `grafana_cloud` scope, and SHALL produce a signed package. It SHALL NOT publish to the ops or prod catalogs, or create Git tags, GitHub releases or documentation releases. Apart from the dev version pin request, it SHALL NOT start or request a deployment of the plugin. Only a manual ops promotion publishes to the ops catalog.

#### Scenario: Catalog targets
- **WHEN** a `main` push is published
- **THEN** the version SHALL exist in the dev catalog with the `grafana_cloud` scope and in no other catalog
- **AND** no Git tag, GitHub release or documentation release SHALL be created by the publication
- **AND** no ops or prod version pin SHALL change

### Requirement: Publication documentation
Maintainer documentation SHALL describe how a `main` commit becomes a dev catalog version and reaches the dev environment, and how a maintainer promotes a dev-published commit to the ops catalog and ops environment. It SHALL cover the version format, scope, gates, version pins and one-time prerequisites, and how to roll dev or ops back to an earlier published version. It SHALL NOT name private infrastructure repositories.

#### Scenario: Maintainer looks up a commit's version
- **WHEN** a maintainer follows the documentation for a merged commit
- **THEN** they SHALL be able to tell whether and under which version it was published to the dev catalog, and whether dev runs it

#### Scenario: Maintainer rolls back dev
- **WHEN** a published version misbehaves in dev
- **THEN** the documentation SHALL describe pinning an earlier published version, and that the next `main` publication replaces that pin unless the faulty commit is reverted

#### Scenario: Maintainer promotes to ops
- **WHEN** a maintainer wants ops to run a commit that dev runs
- **THEN** the documentation SHALL describe dispatching the promotion with that commit, its gates, and reviewing and merging the resulting ops pin pull request

#### Scenario: Maintainer rolls back ops
- **WHEN** a promoted version misbehaves in ops
- **THEN** the documentation SHALL describe promoting an earlier dev-published commit or pinning an earlier ops version

## ADDED Requirements

### Requirement: Manual ops promotion
A maintainer SHALL be able to publish a `main` commit to the ops catalog only by manually dispatching the ops promotion workflow from `main` of the canonical repository with that commit's full SHA. The promotion SHALL rebuild that commit through the same pinned shared plugin workflow and settings as the dev publication, so the ops catalog version is the version the dev catalog received for that commit.

#### Scenario: Dispatch promotes a commit
- **WHEN** a maintainer dispatches the promotion from `main` with commit `<sha>` and its gates pass while `package.json` at `<sha>` declares `0.1.0-alpha.1`
- **THEN** the ops catalog SHALL contain `0.1.0-alpha.1+<sha>` with the `grafana_cloud` scope
- **AND** its packaged `SOURCE_REVISION` SHALL be `<sha>`

#### Scenario: Pushes and pull requests do not promote
- **WHEN** a commit is pushed to `main` or CI runs for a pull request
- **THEN** no version SHALL be published to the ops catalog

#### Scenario: Dispatch outside canonical main does not promote
- **WHEN** the promotion is dispatched from another branch or in a fork
- **THEN** no version SHALL be published to any catalog

#### Scenario: Commit is not a full SHA
- **WHEN** the promotion is dispatched with a branch name, an abbreviated SHA or an empty value
- **THEN** the promotion SHALL fail before publishing

### Requirement: Ops promotion scope
Promotion SHALL target only the ops catalog, with the `grafana_cloud` scope, and SHALL produce a signed package. It SHALL NOT create Git tags, GitHub releases or documentation releases, or, apart from the ops version pin request, start or request a deployment. Promotions SHALL run one at a time, and a new promotion SHALL NOT cancel one that is running.

#### Scenario: Catalog targets
- **WHEN** a commit is promoted
- **THEN** its version SHALL be published to the ops catalog with the `grafana_cloud` scope and to no other catalog
- **AND** no Git tag, GitHub release or documentation release SHALL be created
- **AND** no dev or prod version pin SHALL change

#### Scenario: Promotions in quick succession
- **WHEN** a promotion is dispatched while another promotion is running
- **THEN** the running promotion SHALL complete before the new one starts
- **AND** a newer dispatch SHALL replace a promotion that is still waiting to start

### Requirement: Ops promotion gate
Before publishing, a promotion SHALL require that the CI run for the commit's push to `main` shows every required source check and plugin validation succeeded, and that the dev publication run for that same push succeeded, including its dev catalog publication job. If either is missing or unsuccessful, no version SHALL be published to the ops catalog.

#### Scenario: Dev-published commit passes the gate
- **WHEN** commit `<sha>` passed source CI on `main` and its dev publication succeeded
- **THEN** the promotion SHALL publish `<sha>` to the ops catalog

#### Scenario: Superseded dev publication blocks promotion
- **WHEN** the dev publication of commit `<sha>` was cancelled because a newer `main` push superseded it
- **THEN** the promotion of `<sha>` SHALL fail without publishing

#### Scenario: Failed source CI blocks promotion
- **WHEN** a required source check or plugin validation for commit `<sha>` did not succeed
- **THEN** the promotion of `<sha>` SHALL fail without publishing

#### Scenario: Commit not pushed to main
- **WHEN** commit `<sha>` has no CI or dev publication run for a push to `main`
- **THEN** the promotion of `<sha>` SHALL fail without publishing

### Requirement: Ops version pin request
After a version is published to the ops catalog, the promotion workflow SHALL open a pull request in the deployment repository that pins the ops environment to exactly that version, replacing any earlier ops pin pull request that has not merged. It SHALL request the pin only if the version ends in `+` and the promoted commit's SHA, whether or not that commit is the head of `main`. Failed or skipped promotions SHALL NOT request a pin.

#### Scenario: Promotion requests the ops pin
- **WHEN** commit `<sha>` is promoted to the ops catalog as `0.1.0-alpha.1+<sha>`
- **THEN** a pull request SHALL pin the ops environment to `0.1.0-alpha.1+<sha>`

#### Scenario: Older commit is promoted
- **WHEN** a commit older than the head of `main` is promoted
- **THEN** the ops pin pull request SHALL pin that commit's version

#### Scenario: Newer promotion replaces an open ops pin
- **WHEN** an ops pin pull request has not merged and another version is promoted
- **THEN** the pin pull request SHALL be replaced by one for the newly promoted version

#### Scenario: Failed promotion requests no pin
- **WHEN** the ops catalog publication fails or is skipped
- **THEN** no ops pin pull request SHALL be opened

#### Scenario: Version does not match the commit
- **WHEN** the published version is empty or does not end in `+<sha>` of the promoted commit
- **THEN** the job SHALL fail without requesting a pin

### Requirement: Reviewed ops pin
The ops pin pull request SHALL NOT be set to merge automatically. A person SHALL review and merge it before ops runs the promoted version.

#### Scenario: Ops pin awaits review
- **WHEN** an ops pin pull request is opened and its checks pass
- **THEN** it SHALL stay open until a person merges it
