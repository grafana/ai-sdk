# module-validation-modes Specification

## Purpose

Separate required Gateway candidate-source integration from standalone artifact validation without weakening the SDK-to-Gateway boundary or publication gates.

## Requirements

### Requirement: Explicit Gateway candidate-source integration

The repository SHALL provide a separately selected Go workspace containing Gateway and its local SDK, provider and middleware source dependencies, without adding Gateway to root `go.work`. Required Gateway source build/test/lint/vet and cross-language commands SHALL explicitly select this workspace; SDK-only commands SHALL select root `go.work`, and the SDK-only integration testserver SHALL remain Gateway-free. Required ProviderWire Go-client captures SHALL use candidate root/client source rather than stale published pins. Gateway release/production builds SHALL use `GOWORK=off` and SHALL NOT implicitly select a development workspace.

#### Scenario: Candidate-source Gateway integration
- **WHEN** coordinated source changes affect SDK and Gateway and required checks run
- **THEN** Go tests and cross-language binaries SHALL execute the candidate implementations, not merely report local paths from `go list`

#### Scenario: Isolated Gateway validation
- **WHEN** Gateway artifact validation or a production image build runs
- **THEN** Go SHALL resolve declared versions with `GOWORK=off`, readonly manifests and no workspace substitutions

#### Scenario: Root default workspace
- **WHEN** Go selects root `go.work`
- **THEN** Gateway SHALL be absent from that workspace

### Requirement: Structural boundary enforcement independent of standalone execution

A structural check SHALL retain the AGPL Gateway / Apache SDK license and module boundary: reject Gateway imports, requirements or replacements in tracked source and manifests outside `ai-gateway/`; exclude Gateway from root `go.work` and SDK/Grafana module graphs; and reject root workspace replacements. Independently, a required check SHALL build and test candidate SDK root and Grafana client with Gateway source physically absent, using a copied Gateway-free workspace. Standalone root/client checks MAY run on demand for consumer readiness but SHALL NOT become a source-PR prerequisite.

#### Scenario: Reverse dependency in nested module
- **WHEN** tracked source or a manifest outside `ai-gateway/` references Gateway, including in a nested module
- **THEN** the structural check SHALL fail in any validation mode

#### Scenario: SDK and Grafana source independence
- **WHEN** the independent source-absence check runs
- **THEN** candidate SDK root and Grafana client SHALL build and test with Gateway absent without needing compatibility with an older published root pin

#### Scenario: Structural check without standalone tests
- **WHEN** the structural boundary check runs
- **THEN** it SHALL report boundary violations independently of public-proxy build/test execution

### Requirement: Selectable published-module standalone validation

The repository SHALL retain one-module and all-published-module standalone commands using a public Go proxy, clean module cache, `GOWORK=off`, readonly manifests, dependency download/verification, build/test and no committed local replacements. Inventory SHALL use tracked nested `go.mod` roots and declared paths. Unknown or local-only example/test selection SHALL fail. These commands SHALL be callable on demand; selected Gateway validation SHALL block artifact publication, but all-module standalone results SHALL NOT block ordinary source PRs.

#### Scenario: One published module
- **WHEN** a published module is selected by its registered module path or root
- **THEN** standalone validation SHALL exercise only that module with the same checks used by the all-module command

#### Scenario: All published modules
- **WHEN** the existing all-module entry point runs
- **THEN** it SHALL continue validating every published module including Gateway, providers, middleware, the Grafana client, and the SDK root

#### Scenario: Invalid or local-only selection
- **WHEN** a caller selects an unregistered module or a deliberately local-only example/test module
- **THEN** validation SHALL fail rather than silently skipping it

### Requirement: No premature source-gate switch

Required source checks MAY activate candidate integration only together with a blocking standalone Gateway plus production image-validation path for eligible main/tag publication. Boundary, parity, integration and merged-pin checks SHALL remain blocking source checks; all-module standalone compilation SHALL not block an otherwise valid source PR.

#### Scenario: Source PR with incomplete standalone dependencies
- **WHEN** a source PR passes required candidate-source checks but cannot build against its older merged published pins
- **THEN** it SHALL remain mergeable, while an unready Gateway artifact SHALL NOT publish or deploy
