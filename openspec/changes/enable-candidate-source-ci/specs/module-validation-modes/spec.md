## MODIFIED Requirements

### Requirement: Explicit Gateway candidate-source integration
The repository SHALL provide a separately selected Go workspace containing Gateway and its local SDK, provider, and middleware source dependencies, without adding Gateway to root `go.work`. Required source build/test/lint/vet and integration commands SHALL use that workspace explicitly for Gateway command builds/tests and Gateway-owned ProviderWire cross-language test-server builds; SDK-only commands SHALL use the root Gateway-free workspace. Go client probes intended to demonstrate pinned standalone consumption SHALL remain available as separate diagnostic or artifact evidence, and required cross-language checks SHALL use a candidate-source client rather than fail on stale pinned SDK compilation. The existing SDK-only integration testserver SHALL not acquire a Gateway dependency. Ordinary SDK builds SHALL not select the Gateway workspace implicitly. Gateway release/production builds SHALL explicitly use `GOWORK=off`.

#### Scenario: Candidate-source Gateway integration
- **WHEN** a coordinated source change affects the SDK and Gateway and required integration runs
- **THEN** tests and applicable Go binaries SHALL resolve candidate SDK/provider/middleware source from the selected workspace rather than committed dependency versions
- **AND** a controlled candidate-source change SHALL produce a distinguishable integration result, not only a local module path in `go list`

#### Scenario: Isolated Gateway validation
- **WHEN** the Gateway standalone or release/production build mode runs
- **THEN** Go SHALL resolve declared versions with `GOWORK=off`, readonly manifests, and no workspace substitutions
- **AND** the same controlled source change SHALL NOT affect the isolated result

#### Scenario: Root default workspace
- **WHEN** Go selects the root `go.work` without an explicit Gateway integration mode
- **THEN** the Gateway module SHALL be absent from that workspace

### Requirement: Structural boundary enforcement independent of standalone execution
A structural Gateway boundary check SHALL verify the AGPL Gateway and Apache SDK license/module layout, reject Gateway module-path references in tracked Go files outside `ai-gateway/`, reject Gateway requirements and replacements in tracked non-Gateway module manifests, and exclude Gateway from the root workspace and root SDK and Grafana client module graphs. The root workspace SHALL have no replacements. Standalone build/test checks SHALL remain separate and SHALL validate the SDK root and Grafana client with `GOWORK=off` when explicitly run for artifact or consumer readiness. An independent required source check SHALL build and test both against candidate source with Gateway source absent.

#### Scenario: Reverse dependency in nested module
- **WHEN** a tracked Go file or module outside `ai-gateway/`, including a nested module, references the Gateway module path in source, a requirement, or a replacement
- **THEN** the structural check SHALL fail even if the explicit integration workspace exists

#### Scenario: SDK and Grafana source independence
- **WHEN** standalone validation and the independent source-absent check run
- **THEN** standalone validation SHALL build/test the root SDK and Grafana client with `GOWORK=off`, and neither module graph SHALL contain Gateway
- **AND** the independent check SHALL build/test both from candidate source with Gateway source absent, without requiring stale published-root compatibility

#### Scenario: Structural check without standalone tests
- **WHEN** the structural boundary check runs
- **THEN** it SHALL report boundary violations independently of public-proxy build/test execution

### Requirement: Selectable published-module standalone validation
The repository SHALL provide one-module and all-published-module standalone validation entry points sharing the same policy: public Go proxy, clean module cache, `GOWORK=off`, readonly manifests, dependency download/verification, build/test, and no committed local replacements. Module inventory SHALL use tracked nested `go.mod` ownership and declared module paths. Selection of an unknown or local-only test/example module SHALL fail. The all-module command SHALL remain available for diagnostics and artifact gating, but its failure SHALL NOT block ordinary source PRs directly or via a required aggregate.

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
The candidate-source integration modes SHALL be activated in required source CI only together with a successful standalone Gateway artifact gate for main/tag image publication and deployment. Existing boundary, parity, integration and merged-pin source checks SHALL remain effective. The standalone checks SHALL remain effective for artifact publication but SHALL no longer block an otherwise valid source PR.

#### Scenario: Source PR with incomplete standalone dependencies
- **WHEN** a source PR passes all required candidate-source checks but fails all-module standalone validation against older merged pins
- **THEN** the source PR SHALL remain mergeable and the standalone result SHALL be visible as a nonblocking diagnostic
- **AND** a push/tag of that unready revision SHALL NOT publish or deploy a Gateway image
