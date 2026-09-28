## MODIFIED Requirements

### Requirement: AGPL Gateway repository boundary

The ProviderWire V4 production schema and contract workspace SHALL live under `ai-gateway/`. That directory SHALL be the separate Go module `github.com/grafana/ai-sdk/ai-gateway` and SHALL be licensed under AGPL-3.0-only. Reusable SDK files outside `ai-gateway/` SHALL remain under the root Apache-2.0 license.

The dependency boundary SHALL be one-way: Gateway code MAY import explicitly pinned SDK modules, but no Go source or module outside `ai-gateway/` SHALL import, require, or replace the Gateway module. The Gateway module SHALL remain absent from the root `go.work` and root module graph, and the root SDK SHALL build and test with `GOWORK=off`. A separately named, explicitly selected integration workspace MAY include Gateway and local SDK source for candidate-source testing and Gateway container production builds at the selected checkout revision; its existence SHALL NOT change the root workspace, permit a reverse SDK-to-Gateway dependency, or replace standalone validation for independently consumable SDK/provider/middleware Go-module publication. Gateway's supported release artifact SHALL be its container image, not a standalone Go-module package; its internal module boundary and declared pins SHALL remain subject to existing ancestry and license policy.

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
- **WHEN** the separate Gateway source-integration workspace is selected for a coordinated source test or Gateway container build
- **THEN** the Gateway SHALL resolve local SDK source without adding Gateway to the root `go.work`
- **AND** reverse SDK-to-Gateway references SHALL still fail module-boundary verification
- **AND** independently consumable SDK/provider/middleware Go modules SHALL still require standalone workspace-off release validation

#### Scenario: Image release is not module support
- **WHEN** release-please creates an `ai-gateway/vX.Y.Z` application tag and the Gateway image builds at that tag
- **THEN** the tag SHALL select the source revision for the same workspace-based container recipe
- **AND** discoverability by Go tooling SHALL NOT imply a supported standalone Gateway `go install ...@version` or external implementation-package import contract
