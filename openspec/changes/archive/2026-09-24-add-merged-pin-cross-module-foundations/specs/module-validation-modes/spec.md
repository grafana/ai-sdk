## ADDED Requirements

### Requirement: Explicit Gateway candidate-source integration

The repository SHALL provide a separately selected Go workspace containing Gateway and its local SDK, provider, and middleware source dependencies, without adding Gateway to root `go.work`. Dedicated integration commands SHALL use that workspace explicitly for Gateway command builds/tests and Gateway-owned ProviderWire cross-language test-server builds; Go client probes intended to demonstrate pinned standalone consumption SHALL remain isolated, and the existing SDK-only integration testserver SHALL not acquire a Gateway dependency. Ordinary SDK builds SHALL not select the Gateway workspace implicitly. Gateway release/production builds SHALL explicitly use `GOWORK=off`.

#### Scenario: Candidate-source Gateway integration
- **WHEN** a coordinated source change affects the SDK and Gateway and the Gateway integration mode runs
- **THEN** tests and applicable Go binaries SHALL resolve the candidate SDK/provider/middleware source from the selected workspace rather than committed dependency versions
- **AND** a controlled candidate-source change SHALL produce a distinguishable integration result, not only a local module path in `go list`

#### Scenario: Isolated Gateway validation
- **WHEN** the Gateway standalone or release/production build mode runs
- **THEN** Go SHALL resolve declared versions with `GOWORK=off`, readonly manifests, and no workspace substitutions
- **AND** the same controlled source change SHALL NOT affect the isolated result

#### Scenario: Root default workspace
- **WHEN** Go selects the root `go.work` without an explicit Gateway integration mode
- **THEN** the Gateway module SHALL be absent from that workspace

### Requirement: Structural boundary enforcement independent of standalone execution

A structural Gateway boundary check SHALL verify the AGPL Gateway and Apache SDK license/module layout, reject Gateway module-path references in tracked Go files outside `ai-gateway/`, reject Gateway requirements and replacements in tracked non-Gateway module manifests, and exclude Gateway from the root workspace and root SDK and Grafana client module graphs. The root workspace SHALL have no replacements. Standalone build/test checks SHALL remain separate and SHALL validate the SDK root and Grafana client with `GOWORK=off`.

#### Scenario: Reverse dependency in nested module
- **WHEN** a tracked Go file or module outside `ai-gateway/`, including a nested module, references the Gateway module path in source, a requirement, or a replacement
- **THEN** the structural check SHALL fail even if the explicit integration workspace exists

#### Scenario: Standalone SDK and Grafana validation
- **WHEN** the all-module standalone validation runs
- **THEN** the root SDK and Grafana client SHALL build and test with `GOWORK=off` and neither module graph SHALL contain Gateway

#### Scenario: Structural check without standalone tests
- **WHEN** the structural boundary check runs
- **THEN** it SHALL report boundary violations independently of public-proxy build/test execution

### Requirement: Selectable published-module standalone validation

The repository SHALL provide one-module and all-published-module standalone validation entry points sharing the same policy: public Go proxy, clean module cache, `GOWORK=off`, readonly manifests, dependency download/verification, build/test, and no committed local replacements. Module inventory SHALL use tracked nested `go.mod` ownership and declared module paths. Selection of an unknown or local-only test/example module SHALL fail. The existing all-module command and required CI enforcement SHALL remain intact.

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

The new integration and scoped validation modes SHALL remain opt-in tooling until the separate workflow transition activates them. Existing required standalone, boundary, parity, integration, Gateway image, publication, and deployment checks SHALL remain effective.

#### Scenario: Source PR with incomplete standalone dependencies
- **WHEN** a source PR passes candidate-source integration but fails the existing all-module standalone gate
- **THEN** the source PR SHALL remain blocked by that gate in this change
