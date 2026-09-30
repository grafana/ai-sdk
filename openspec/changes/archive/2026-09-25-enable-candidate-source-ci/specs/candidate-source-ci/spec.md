## ADDED Requirements

### Requirement: Required CI integrates candidate source
Required source CI SHALL build, vet, lint, test and exercise cross-language integration against candidate SDK, providers, middleware and Gateway. It SHALL retain formatting, docs, license/isolation, parity/conformance, structural boundary, module-policy and canonical merged-pin checks. Real published internal pins MAY lag candidate source but SHALL remain downloadable and merged in canonical main; required source checks SHALL NOT compile candidate code against those pins as a prerequisite to merge.

#### Scenario: Coordinated source with older merged pins
- **WHEN** candidate root/provider/middleware/Gateway changes work together but an older merged dependency pin cannot compile that candidate independently
- **THEN** required source checks SHALL execute and validate the candidate modules without requiring a standalone build to pass
- **AND** unmerged or unverifiable pins, forbidden reverse dependencies and real source regressions SHALL still fail their required checks

#### Scenario: ProviderWire candidate differential
- **WHEN** the required ProviderWire client differential and request-mutation controls run
- **THEN** the Go capture SHALL build and run candidate Grafana client against candidate SDK root, retaining the pinned TypeScript comparator
- **AND** every copied Go-client mutant SHALL be selected by its test workspace and rejected by a semantic differential assertion, not a build/setup failure

### Requirement: Gateway publication is independently gated
Standalone selected-module and all-module commands SHALL remain available without directly or indirectly blocking an ordinary source PR. On an eligible canonical main or Gateway-tag push, image validation SHALL first test the Gateway module at the exact checkout revision with public-proxy dependencies, a clean module cache, readonly module files, `GOWORK=off` and no local replacements. Only after that succeeds MAY it build and smoke-test standalone production images. Publication SHALL depend on successful required source and image-validation jobs for that revision; main deployment SHALL follow successful publication. A skipped, cancelled or failed image-validation job SHALL NOT authorize publication.

#### Scenario: Unready Gateway artifact
- **WHEN** source checks pass but the Gateway module cannot build or test with its declared published dependencies
- **THEN** the source PR SHALL remain eligible to merge and an artifact push SHALL NOT publish or deploy that Gateway image

#### Scenario: Valid artifact
- **WHEN** required source checks, Gateway standalone validation and production image validation all pass for the same eligible push revision
- **THEN** publication MAY proceed, and main deployment MAY follow successful publication

### Requirement: Release automation remains separate
A green source PR SHALL NOT authorize an SDK release or change required-check/ruleset settings. Any required-check identity change SHALL require coordinated maintainer approval without bypassing checks. SDK release automation remains owned by #245/#21.

#### Scenario: Source PR passes
- **WHEN** a coordinated source PR passes required checks
- **THEN** no SDK tag or module publication SHALL be created by this workflow
