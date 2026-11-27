# candidate-source-ci Specification

## Purpose

Validate coordinated candidate SDK and Gateway source independently of standalone artifact readiness, while keeping publication fail-closed.

## Requirements

### Requirement: Required CI integrates candidate source

Required source CI SHALL build, vet, lint, test and exercise cross-language integration against candidate SDK, providers, middleware and Gateway through each module's local first-party replacements. It SHALL retain formatting, docs, license/isolation, parity/conformance, structural boundary, module-policy and manifest-aligned requirement checks. Compiling candidate code against published first-party versions SHALL NOT be a merge prerequisite.

#### Scenario: Coordinated source with older merged pins

- **WHEN** candidate root/provider/middleware/Gateway changes work together but no published first-party version contains the new behavior
- **THEN** required source checks SHALL execute and validate the candidate modules through local replacements
- **AND** requirements that differ from the manifest, forbidden reverse dependencies and real source regressions SHALL still fail their required checks

#### Scenario: ProviderWire candidate differential

- **WHEN** the required ProviderWire client differential and request-mutation controls run
- **THEN** the Go capture SHALL build and run candidate Grafana client against candidate SDK root, retaining the pinned TypeScript comparator
- **AND** every copied Go-client mutant SHALL be selected by its test harness and rejected by a semantic differential assertion, not a build/setup failure

### Requirement: Required CI race-tests the Gateway stream path

Required source CI SHALL run the short tests with the Go race detector for the root module, the Gateway, and the provider and middleware modules on the Gateway stream path: anthropic, openai, openai-compatible, logger, prometheus and agentobservability.

#### Scenario: Data race in a stream-path module

- **WHEN** a short test in one of those modules triggers a data race
- **THEN** the race job SHALL fail and Gateway publication SHALL remain blocked

### Requirement: Gateway publication is independently gated

Gateway compilation against published first-party versions SHALL NOT block plugin release or main deployment. Gateway publication SHALL run only for an eligible push after that push's required CI run shows its source, manifest-aligned requirement, boundary and plugin-validation jobs succeeded for that revision. Publication SHALL NOT run inside, delay or change the conclusion of the required CI run. Deployment SHALL follow publication.

#### Scenario: Unready Gateway artifact

- **WHEN** coordinated source checks pass but Gateway could not compile against published first-party versions
- **THEN** the source PR SHALL remain eligible to merge and plugin publication MAY proceed only if the plugin package built from same-revision source, its validation and the protocol test host suites at that push SHA pass
- **AND** SDK/provider/middleware module releases SHALL still require their own release evidence

#### Scenario: Valid artifact

- **WHEN** required source checks, boundary and requirement checks, plugin package validation and protocol test host suites all pass for the same eligible push revision
- **THEN** publication MAY proceed using that revision's source, and main deployment MAY follow successful publication

#### Scenario: Failed source, image or smoke check

- **WHEN** a required candidate-source check, build, plugin validation, requirement or boundary check fails, is skipped or is cancelled
- **THEN** Gateway publication and deployment SHALL remain blocked

#### Scenario: Publication without CI evidence

- **WHEN** publication would run for a pull request, or before the push's required CI run shows every required source and plugin-validation job succeeded for that commit
- **THEN** no publication SHALL occur

#### Scenario: Release evidence is unaffected

- **WHEN** a Gateway publication runs, waits or is superseded for a `main` push
- **THEN** that push's required CI run SHALL complete and conclude exactly as it would without publication

### Requirement: Gateway publication fails closed on job state

A skipped, cancelled or failed required source or plugin-validation job SHALL NOT authorize publication or deployment.

#### Scenario: Gateway publication fails closed on job state
- **WHEN** a plugin-validation job is cancelled
- **THEN** Gateway publication and main deployment SHALL remain unauthorized

### Requirement: Release automation remains separate
A green source PR SHALL NOT authorize an SDK release or change required-check/ruleset settings. Any required-check identity change SHALL require coordinated maintainer approval without bypassing checks. SDK release automation remains owned by #245/#21.

#### Scenario: Source PR passes
- **WHEN** a coordinated source PR passes required checks
- **THEN** no SDK tag or module publication SHALL be created by this workflow

### Requirement: Gateway publication checkout and plugin validation

Plugin validation SHALL run on pull requests and eligible pushes. On eligible canonical main or Gateway-tag pushes, plugin validation SHALL use a fresh clean checkout, verify HEAD equals the push SHA with standard Git cleanliness checks, build the plugin backend from `ai-gateway/go.mod` with `GOWORK=off`, check generated manifest consistency and run plugin package validation.

#### Scenario: Gateway publication checkout and plugin validation

- **WHEN** plugin validation runs for an eligible Gateway-tag push
- **THEN** the clean push-SHA checkout SHALL build and validate the plugin package from same-revision source and retain it as the validated artifact

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
- **THEN** both suites SHALL share one ordinary Gateway and host build from the candidate's local module graph, with separate test processes and fixtures
- **AND** Go stack-catalog scenarios SHALL use a distinct race-enabled, test-only Gateway build with the same host
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
