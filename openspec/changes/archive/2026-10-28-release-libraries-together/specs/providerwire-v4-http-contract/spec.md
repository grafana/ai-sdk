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
- **AND** SDK module build graphs SHALL exclude it
- **AND** the root SDK SHALL build and test with `GOWORK=off`

#### Scenario: Explicit integration workspace does not cross the boundary
- **WHEN** a coordinated source test or Gateway plugin backend build resolves local SDK source through the replacements in `ai-gateway/go.mod`
- **THEN** no SDK module SHALL gain a requirement or replacement of Gateway
- **AND** reverse SDK-to-Gateway references SHALL still fail module-boundary verification

#### Scenario: Image release is not module support
- **WHEN** release-please creates an `ai-gateway/vX.Y.Z` application tag and the Gateway plugin package builds at that tag
- **THEN** the tag SHALL select the source revision for the same-revision plugin build
- **AND** discoverability by Go tooling SHALL NOT imply a supported standalone Gateway command, `go install ...@version` or external implementation-package import contract

### Requirement: One-way Gateway module dependencies

The dependency boundary SHALL be one-way: Gateway code MAY import SDK modules, which it resolves through local replacements, but no Go source or module outside `ai-gateway/` SHALL import, require, or replace the Gateway module. The Gateway module SHALL remain absent from SDK module graphs, and the root SDK SHALL build and test with `GOWORK=off`.

#### Scenario: One-way Gateway module dependencies
- **WHEN** root SDK verification runs with GOWORK=off
- **THEN** Gateway SHALL remain outside the root module graph and no external source/module SHALL import, require or replace it

### Requirement: Gateway plugin release and pin policy
The Gateway's supported release artifact SHALL be its App Platform plugin package, not a container image or standalone Go-module package. Its internal module boundary and declared first-party requirements SHALL remain subject to the manifest-aligned requirement and license policy.

#### Scenario: Gateway plugin release and pin policy
- **WHEN** an ai-gateway version tag selects a release artifact
- **THEN** the supported artifact SHALL be the plugin package, with first-party requirements subject to the manifest-aligned requirement and license policy rather than standalone module support

## REMOVED Requirements

### Requirement: Explicit candidate-source integration workspace boundary

**Reason**: The separate Gateway workspace is removed; Gateway resolves same-revision SDK source through the local replacements in its own `go.mod`.

**Migration**: See "One-way Gateway module dependencies".
