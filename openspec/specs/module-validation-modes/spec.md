# module-validation-modes Specification

## Purpose

Separate required Gateway candidate-source integration from standalone artifact validation without weakening the SDK-to-Gateway boundary or publication gates.

## Requirements

### Requirement: Structural boundary enforcement independent of standalone execution

Structural checks SHALL enforce AGPL Gateway/Apache SDK boundaries: reject Gateway imports, requirements or replacements in tracked source/manifests outside `ai-gateway/` and exclude Gateway from SDK and Grafana client build graphs. An independent required check SHALL build and test candidate SDK root and Grafana client with Gateway physically absent in a repository copy without `ai-gateway/`.

#### Scenario: Reverse dependency in nested module

- **WHEN** tracked source or a manifest outside `ai-gateway/` references Gateway, including in a nested module
- **THEN** the structural check SHALL fail in any validation mode

#### Scenario: SDK and Grafana source independence

- **WHEN** the independent source-absence check runs
- **THEN** candidate SDK root and Grafana client SHALL build and test with Gateway absent through their local replacements

#### Scenario: Structural check without standalone tests

- **WHEN** the structural boundary check runs
- **THEN** it SHALL report boundary violations independently of build/test execution

### Requirement: Same-revision first-party source through local replacements

Every tracked `go.mod` SHALL replace each first-party module in its build graph,
including transitive ones, with that module's repository directory. The
repository SHALL NOT commit `go.work`, `go.gateway.work` or their sum files.
Required build, vet, lint, test and cross-language commands SHALL run inside each
module with `GOWORK=off` and readonly manifests, so they execute same-revision
candidate source. Required ProviderWire Go-client captures SHALL use candidate
root and client source.

#### Scenario: Coordinated change across modules

- **WHEN** a pull request changes root and a provider that uses the new root behavior
- **THEN** the provider builds and tests against the candidate root source without a published root version containing it

#### Scenario: Missing transitive replacement

- **WHEN** a module's build graph contains a first-party module that its `go.mod` does not replace with the registered directory
- **THEN** the module-policy check fails and identifies the module and missing replacement

#### Scenario: Ambient workspace

- **WHEN** a developer has an uncommitted `go.work` or `GOWORK` set in the environment
- **THEN** required tasks and CI still resolve first-party modules through each module's local replacements

### Requirement: Per-module validation

The repository SHALL provide one-module and all-module commands that download,
verify, build and test each tracked module with `GOWORK=off`, readonly manifests
and only its local first-party replacements. Inventory SHALL use tracked `go.mod`
roots and declared paths; an unknown selection SHALL fail.

#### Scenario: One module

- **WHEN** a tracked module is selected by its registered module path or root
- **THEN** validation exercises only that module with the same checks used by the all-module command

#### Scenario: Invalid selection

- **WHEN** a caller selects an unregistered module
- **THEN** validation fails rather than silently skipping it

### Requirement: Gateway plugin backend builds from its module

Gateway plugin backend, release and production builds SHALL build
`ai-gateway/go.mod` at the selected checkout revision with `GOWORK=off` and
readonly manifests, resolving SDK, provider and middleware source through its
local replacements. Ambient development workspaces SHALL NOT determine backend
dependencies.

#### Scenario: Explicit Gateway dependencies

- **WHEN** a Gateway production plugin backend builds with a different ambient development workspace
- **THEN** its dependencies still come from the local replacements in `ai-gateway/go.mod` at the selected revision
