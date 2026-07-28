# upstream-parity-governance Specification

## Purpose

Define repository standards for declaring, checking, upgrading, and reviewing
parity with the upstream Vercel AI SDK baseline.

## Requirements

### Requirement: Upstream parity baseline manifest
The repository SHALL define a checked-in upstream parity baseline manifest that records the verified upstream AI SDK target for parity-sensitive work. The manifest SHALL include the upstream repository URL, the coherent TypeScript package versions used by all parity consumers and reference tooling, the verification status, the verification commands, and known intentional deviations or accepted gaps. Directly consumed packages and transitive packages used as authoritative parity references SHALL be registered so compatibility and upgrade workflows can enforce one coherent package set.

#### Scenario: Contributor finds the verified upstream target
- **WHEN** a contributor needs to know which upstream AI SDK version this repository is verified against
- **THEN** the upstream parity baseline manifest identifies the upstream repository and package versions used for verification

#### Scenario: Baseline records known gaps
- **WHEN** the repository intentionally diverges from upstream behavior or lacks coverage for an upstream surface
- **THEN** the upstream parity baseline manifest records the deviation or gap with enough context for reviewers to classify it as intentional

### Requirement: Parity coverage map
The repository SHALL maintain a parity coverage map that classifies compatibility surfaces by verification status, layer, confidence source, and known gaps. The coverage map SHALL cover at minimum core orchestration and UI chunk behavior, provider contract compatibility, provider implementation behavior, frontend interop, conformance harness capabilities, and known gaps. Each surface SHALL be classified as automated, manual, documented deviation, mixed coverage, or gap.

#### Scenario: Reviewer evaluates compatibility coverage
- **WHEN** a reviewer inspects a change touching a parity-sensitive surface
- **THEN** the coverage map identifies the affected layer and whether that surface is protected by automated conformance, manual review, documented deviation, mixed coverage, or an explicit gap

#### Scenario: New parity surface is added
- **WHEN** implementation adds a new upstream compatibility surface
- **THEN** the coverage map is updated to classify how the new surface is verified

#### Scenario: Provider implementation work is reviewed
- **WHEN** implementation changes provider request conversion, provider response parsing, provider-defined tools, or provider options
- **THEN** the coverage map identifies the provider implementation capability and whether request snapshots, stream snapshots, manual review, or a documented gap provide confidence

#### Scenario: Core ai-sdk work is reviewed
- **WHEN** implementation changes orchestration, stream part conversion, UI chunk output, tools, or structured output
- **THEN** the coverage map identifies the core capability and whether UI chunk snapshots, structured output snapshots, manual review, or a documented gap provide confidence

### Requirement: Parity check command
The repository SHALL provide a standard parity check command that validates the upstream parity baseline and runs the relevant automated compatibility checks. The command SHALL validate manifest consistency with every parity TypeScript consumer dependency pin, typecheck conformance tooling, and run conformance tests or a documented stable conformance subset.

#### Scenario: Developer checks current parity
- **WHEN** a developer runs the parity check command
- **THEN** the command validates baseline metadata consistency and executes the configured automated parity checks

#### Scenario: Baseline package drift
- **WHEN** the manifest declares a TypeScript package version that differs from a parity consumer dependency pin
- **THEN** the parity check command fails with a diagnostic identifying the consumer and mismatched package

#### Scenario: Upgrade candidate package set is incoherent
- **WHEN** a parity upgrade candidate declares conflicting exact stable dependency versions for another tracked AI SDK package
- **THEN** the upgrade workflow rejects that candidate instead of selecting incompatible parity reference stacks

### Requirement: Parity upgrade workflow
The repository SHALL provide a documented parity upgrade workflow for moving the registered upstream baseline forward. The workflow SHALL upgrade tracked dependencies across all parity TypeScript consumers to a coherent set of stable npm `latest` versions, regenerate expected outputs and request snapshots, run parity checks, classify failures, and update the baseline manifest only when divergences are fixed or documented as accepted gaps.

#### Scenario: Upstream baseline is upgraded
- **WHEN** a contributor upgrades the registered upstream baseline
- **THEN** the workflow updates the manifest and dependency pins to the latest stable npm `latest` versions, regenerates conformance snapshots, and includes any implementation or documentation needed to satisfy the new baseline

#### Scenario: Upgrade reveals divergence
- **WHEN** regenerated conformance snapshots or parity checks reveal a behavior mismatch
- **THEN** the mismatch is classified as an implementation bug, an intentional deviation, or a coverage gap before the upgrade is considered complete

### Requirement: Agent parity guidance
Repository agent guidance SHALL define parity-sensitive work and the required review posture for that work. The guidance SHALL require upstream source or test comparison during planning for parity-sensitive changes, conformance fixture consideration for wire or provider-boundary behavior, and explicit classification of differences as parity-preserving Go adaptation, intentional deviation, or bug.

#### Scenario: Agent works on parity-sensitive code
- **WHEN** an agent changes stream parts, UI chunks, provider messages, provider request conversion, tool orchestration, output behavior, provider options, or frontend interop behavior
- **THEN** the agent guidance requires upstream comparison and conformance consideration before implementation is considered complete

#### Scenario: No upstream equivalent exists
- **WHEN** a change has no direct upstream TypeScript equivalent
- **THEN** the agent guidance requires the absence of upstream equivalent to be stated and the compatibility impact to be classified

#### Scenario: Reported bug can be captured by conformance
- **WHEN** a reported bug can be represented by recorded provider chunks, provider request snapshots, or structured output snapshots
- **THEN** the agent guidance recommends adding or updating the conformance fixture first, observing the Go replay failure, and then implementing the fix

#### Scenario: New parity-sensitive feature is implemented
- **WHEN** a new feature affects upstream-visible provider or UI behavior
- **THEN** the agent guidance recommends recording or importing upstream behavior alongside the implementation so conformance acts as the regression contract

### Requirement: Parity workflow skills
The repository SHALL provide repo-local Codex skills for scoped parity review and parity upgrade workflows. Each skill SHALL direct agents to read the upstream parity baseline first, inspect relevant upstream source or tests, run focused verification commands, and report bugs, intentional deviations, and coverage gaps separately.

#### Scenario: Agent performs scoped parity review
- **WHEN** the parity review skill is used for a PR, current git diff, branch range, package, directory, provider, bug report, or feature area
- **THEN** the agent reads the baseline manifest, resolves the requested scope, compares relevant behavior against the registered upstream target, runs focused parity checks when useful, and reports only problematic classified findings

#### Scenario: Agent performs parity upgrade
- **WHEN** the parity upgrade skill is used
- **THEN** the agent follows the documented upgrade workflow and does not mark the baseline upgrade complete until divergences are fixed or documented

#### Scenario: Agent reviews a broad scope
- **WHEN** the parity review scope is broader than a diff, such as a provider package or full directory
- **THEN** the agent prioritizes missing features, behavioral deviations, possible bugs, uncovered cases, and undocumented intentional deviations instead of listing every parity-preserving difference

### Requirement: Baseline validation covers all parity TypeScript consumers
The repository SHALL validate that every test package consuming `ai` or `@ai-sdk/*` packages uses versions compatible with the registered upstream parity baseline.

#### Scenario: Integration package consumes upstream AI SDK packages
- **WHEN** `test/integration/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

#### Scenario: CLI package consumes upstream AI SDK packages
- **WHEN** `test/cli/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

#### Scenario: Interop package consumes upstream AI SDK packages
- **WHEN** `test/interop/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

#### Scenario: Parity upgrade updates every consumer
- **WHEN** the registered package set is upgraded
- **THEN** conformance, integration, interop, and CLI package manifests that consume a tracked package are updated together

### Requirement: Provider API-shape drift report
The repository SHALL provide a provider V4 API-shape drift report that compares upstream LanguageModelV4 discriminator values with Go provider constants. By default, the report SHALL resolve the `@ai-sdk/provider` source version declared in the upstream parity baseline. An explicitly supplied source root MAY override that source, but the report SHALL NOT silently fall back to an arbitrary local checkout.

#### Scenario: Upstream adds a stream part type
- **WHEN** upstream LanguageModelV4 declares a stream part discriminator missing from Go provider constants
- **THEN** the drift report identifies the missing value as parity review input

#### Scenario: Registered provider source is selected
- **WHEN** the drift report runs without an explicit source-root override
- **THEN** it reads `@ai-sdk/provider` from the registered baseline and compares against that installed package source

#### Scenario: Registered provider source is unavailable
- **WHEN** the registered provider package source is not installed
- **THEN** the drift report reports that the registered source is unavailable and does not substitute another upstream version silently

### Requirement: Frontend hook interop coverage
The repository SHALL include hook-level frontend interop tests for upstream AI SDK UI consumers.

#### Scenario: React chat hook consumes Go UI stream
- **WHEN** integration tests run
- **THEN** a `useChat`-level test verifies the Go SSE stream can be consumed through the upstream React hook surface

#### Scenario: React object and completion surfaces consume Go streams
- **WHEN** integration tests run
- **THEN** hook-level or equivalent upstream UI tests cover object and completion stream consumption
