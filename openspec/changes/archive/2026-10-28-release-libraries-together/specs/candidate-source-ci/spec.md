## MODIFIED Requirements

### Requirement: Required CI integrates candidate source

Required source CI SHALL build, vet, lint, test and exercise cross-language integration against candidate SDK, providers, middleware and Gateway through each module's local first-party replacements. It SHALL retain formatting, docs, license/isolation, parity/conformance, structural boundary, module-policy and manifest-aligned requirement checks. It SHALL run the short tests with the Go race detector for the root module, the Gateway, and the provider and middleware modules on the Gateway stream path: anthropic, openai, openai-compatible, logger, prometheus and agentobservability. Compiling candidate code against published first-party versions SHALL NOT be a merge prerequisite.

#### Scenario: Coordinated source with older merged pins

- **WHEN** candidate root/provider/middleware/Gateway changes work together but no published first-party version contains the new behavior
- **THEN** required source checks SHALL execute and validate the candidate modules through local replacements
- **AND** requirements that differ from the manifest, forbidden reverse dependencies and real source regressions SHALL still fail their required checks

#### Scenario: ProviderWire candidate differential

- **WHEN** the required ProviderWire client differential and request-mutation controls run
- **THEN** the Go capture SHALL build and run candidate Grafana client against candidate SDK root, retaining the pinned TypeScript comparator
- **AND** every copied Go-client mutant SHALL be selected by its test harness and rejected by a semantic differential assertion, not a build/setup failure

#### Scenario: Data race in a stream-path module

- **WHEN** a short test in one of those modules triggers a data race
- **THEN** the race job SHALL fail and Gateway publication SHALL remain blocked

### Requirement: Gateway publication is independently gated

Gateway compilation against published first-party versions SHALL NOT block plugin release or main deployment. Until a Gateway plugin publication workflow exists, CI SHALL NOT publish or deploy the Gateway. Any publication workflow SHALL require the source, manifest-aligned requirement, boundary and plugin-validation jobs to pass for that revision, and deployment SHALL follow publication.

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

### Requirement: Gateway publication checkout and plugin validation

Plugin validation SHALL run on pull requests and eligible pushes. On eligible canonical main or Gateway-tag pushes, plugin validation SHALL use a fresh clean checkout, verify HEAD equals the push SHA with standard Git cleanliness checks, build the plugin backend from `ai-gateway/go.mod` with `GOWORK=off`, check generated manifest consistency and run plugin package validation.

#### Scenario: Gateway publication checkout and plugin validation

- **WHEN** plugin validation runs for an eligible Gateway-tag push
- **THEN** the clean push-SHA checkout SHALL build and validate the plugin package from same-revision source and retain it as the validated artifact
