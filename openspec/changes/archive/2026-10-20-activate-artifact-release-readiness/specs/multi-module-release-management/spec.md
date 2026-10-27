## ADDED Requirements

### Requirement: Authentic library release candidate

Library readiness SHALL verify immutable release bot and repository identities,
one component version, matching title/body/manifest versions, and an exclusively
generated regular-file footprint with preserved changelog history. Release-looking
PRs without a manifest update SHALL fail. Ordinary source PRs SHALL retain their
candidate-source gates without requiring publication readiness.

#### Scenario: Inconsistent generated release

- **WHEN** release metadata disagrees with the generated version or changelog
- **THEN** library readiness fails without authorizing publication

#### Scenario: Release without a version update

- **WHEN** a release-looking PR leaves the version manifest unchanged
- **THEN** it fails rather than being classified as an ordinary source PR

### Requirement: Current library candidate validation

Library readiness SHALL require a clean checkout merging the exact live release
head and current canonical base. It SHALL recheck live metadata and canonical main
after validation and reject changes during the run. A successful check SHALL NOT
replace required-check/current-base protection or post-merge authorization.

#### Scenario: Main advances during validation

- **WHEN** canonical main advances while library readiness runs
- **THEN** the stale candidate fails the final current-base check

#### Scenario: Candidate changes during validation

- **WHEN** the release head or release metadata changes while readiness runs
- **THEN** the run fails and requires validation of the new candidate

### Requirement: Canonical prerequisite tag identity

Library prerequisites SHALL resolve from the public proxy without replacements,
use tagged versions, match canonical Git origin and canonical tag targets, and
be ancestors of the candidate base. Missing or unverifiable evidence SHALL fail
readiness before standalone or consumer success can authorize a library.

#### Scenario: Published version has the wrong tag target

- **WHEN** a prerequisite download origin differs from its canonical tag target
- **THEN** library readiness rejects the prerequisite

#### Scenario: Prerequisite cannot be verified

- **WHEN** a prerequisite is a pseudo-version, unmerged, replaced, unavailable,
  or has an unverifiable origin
- **THEN** library readiness fails closed
