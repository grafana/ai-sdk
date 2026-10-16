# Spec Delta

## MODIFIED Requirements

### Requirement: AGPL Gateway repository boundary

The ProviderWire V4 production schema and contract workspace SHALL live under `ai-gateway/`. That directory SHALL be the separate Go module `github.com/grafana/ai-sdk/ai-gateway` and SHALL be licensed under AGPL-3.0-only. Reusable SDK files outside `ai-gateway/` SHALL remain under the root Apache-2.0 license.

#### Scenario: Gateway artifacts use the Gateway module
- **WHEN** the ProviderWire V4 schema and contract workspace are inspected
- **THEN** they SHALL reside under `ai-gateway/`
- **AND** `ai-gateway/go.mod` SHALL declare `github.com/grafana/ai-sdk/ai-gateway`
- **AND** the nearest license SHALL be AGPL-3.0-only

#### Scenario: SDK modules remain independent
- **WHEN** module-boundary verification runs
- **THEN** no Go source or module outside `ai-gateway/` SHALL import, require, or replace the Gateway module
- **AND** the root `go.work` and root module graph SHALL exclude it
- **AND** the root SDK SHALL build and test with `GOWORK=off`

#### Scenario: Explicit integration workspace does not cross the boundary
- **WHEN** the separate Gateway source-integration workspace is selected for a coordinated source test or Gateway plugin backend build
- **THEN** the Gateway SHALL resolve local SDK source without adding Gateway to the root `go.work`
- **AND** reverse SDK-to-Gateway references SHALL still fail module-boundary verification
- **AND** independently consumable SDK/provider/middleware Go modules SHALL still require standalone workspace-off release validation

#### Scenario: Image release is not module support
- **WHEN** release-please creates an `ai-gateway/vX.Y.Z` application tag and the Gateway plugin package builds at that tag
- **THEN** the tag SHALL select the source revision for the same workspace-based plugin build
- **AND** discoverability by Go tooling SHALL NOT imply a supported standalone Gateway command, `go install ...@version` or external implementation-package import contract

### Requirement: Explicit candidate-source integration workspace boundary

A separately named, explicitly selected integration workspace MAY include Gateway and local SDK source for candidate-source testing and Gateway plugin backend builds at the selected checkout revision; its existence SHALL NOT change the root workspace, permit a reverse SDK-to-Gateway dependency, or replace standalone validation for independently consumable SDK/provider/middleware Go-module publication.

#### Scenario: Explicit candidate-source integration workspace boundary
- **WHEN** a Gateway plugin backend build selects the separate integration workspace
- **THEN** local SDK source MAY be selected without root-workspace changes, reverse dependencies or replacing standalone SDK/provider/middleware publication validation

## ADDED Requirements

### Requirement: Gateway plugin release and pin policy
The Gateway's supported release artifact SHALL be its App Platform plugin package, not a container image or standalone Go-module package. Its internal module boundary and declared pins SHALL remain subject to existing ancestry and license policy.

#### Scenario: Gateway plugin release and pin policy
- **WHEN** an ai-gateway version tag selects a release artifact
- **THEN** the supported artifact SHALL be the plugin package, with internal module pins subject to ancestry and license policy rather than standalone module support

## REMOVED Requirements

### Requirement: Gateway image release and pin policy
**Reason**: The container image is no longer a Gateway release artifact.
**Migration**: Use "Gateway plugin release and pin policy".
