## MODIFIED Requirements

### Requirement: Required CI integrates candidate source

Required source CI SHALL build, vet, lint, test and exercise cross-language integration against candidate SDK, providers, middleware and Gateway. It SHALL retain formatting, docs, license/isolation, parity/conformance, structural boundary, module-policy and canonical merged-pin checks. It SHALL run the short tests with the Go race detector for the root module, the Gateway, and the provider and middleware modules on the Gateway stream path: anthropic, openai, openai-compatible, logger, prometheus and agentobservability. Real published internal pins MAY lag candidate source but SHALL remain downloadable and merged in canonical main; compiling candidate code against those pins SHALL NOT be a merge prerequisite.

#### Scenario: Coordinated source with older merged pins
- **WHEN** candidate root/provider/middleware/Gateway changes work together but an older merged dependency pin cannot compile that candidate independently
- **THEN** required source checks SHALL execute and validate the candidate modules without requiring a standalone build to pass
- **AND** unmerged or unverifiable pins, forbidden reverse dependencies and real source regressions SHALL still fail their required checks

#### Scenario: ProviderWire candidate differential
- **WHEN** the required ProviderWire client differential and request-mutation controls run
- **THEN** the Go capture SHALL build and run candidate Grafana client against candidate SDK root, retaining the pinned TypeScript comparator
- **AND** every copied Go-client mutant SHALL be selected by its test workspace and rejected by a semantic differential assertion, not a build/setup failure

#### Scenario: Data race in a stream-path module
- **WHEN** a short test in one of those modules triggers a data race
- **THEN** the race job SHALL fail and source CI SHALL remain unsuccessful

## REMOVED Requirements

### Requirement: Gateway publication is independently gated
**Reason**: Automatic Gateway image publication is retired.
**Migration**: Keep source checks and standalone library release validation; no replacement publication path is introduced.

### Requirement: Gateway publication checkout and image validation
**Reason**: CI no longer builds or publishes Gateway images.
**Migration**: Local Docker recipes remain available; future application publication is separately owned.

### Requirement: Gateway publication fails closed on job state
**Reason**: The dependent image publisher is removed with its validation gate.
**Migration**: Source success grants no publication authorization.

## ADDED Requirements

### Requirement: Source CI excludes retired Gateway image automation

Source CI SHALL retain the seven jobs ci, race, docs-lint, module-resolution, parity-baseline, integration-test and conformance-test. It SHALL NOT build, validate or publish Gateway images or trigger on Gateway image tags. Retiring images SHALL NOT weaken release authorization or activate publication.

#### Scenario: Main push or source pull request
- **WHEN** a main push or source pull request triggers CI
- **THEN** all seven source jobs run without an image build, smoke test or publisher

#### Scenario: Gateway tag push
- **WHEN** an ai-gateway/vX.Y.Z tag is pushed
- **THEN** the source CI workflow does not run for that tag

#### Scenario: Historical source workflow
- **WHEN** a workflow started before this removal remains incomplete
- **THEN** this change does not alter or cancel that run or authorize release from incomplete evidence
