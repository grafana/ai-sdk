# gateway-plugin-packaging Specification

## Purpose
Build, license and validate the Gateway app plugin package from the `ai-gateway/` directory using same-revision workspace source for its backend.

## Requirements

### Requirement: Plugin package from the Gateway directory
The `ai-gateway/` directory SHALL be the plugin root. It SHALL contain the plugin metadata, a frontend with an overview page, the app manifest source and its committed generated outputs. The backend SHALL be built from the Gateway command package for Linux amd64 and arm64 at minimum.

#### Scenario: Package contents
- **WHEN** the plugin package is built
- **THEN** it SHALL contain the plugin metadata, frontend bundle, app manifest and backend executables for each supported platform

### Requirement: Generated manifest consistency
The generated manifest data and code SHALL be committed and regenerated from the manifest source with a pinned generator version. Validation SHALL fail when regeneration changes committed output.

#### Scenario: Stale generated manifest
- **WHEN** the manifest source changes without regenerated outputs
- **THEN** validation SHALL fail before packaging

### Requirement: Packaged licensing and source attribution
The package SHALL include the Gateway AGPL license and notice, applicable Apache licensing for linked SDK modules, a per-platform inventory of linked Go modules with their license and notice files, and the licenses of bundled frontend dependencies. Packages built by CI SHALL be attributed to their verified source revision; local builds SHALL be marked as unverified development builds.

#### Scenario: CI package attribution
- **WHEN** a package is built by CI from a verified clean checkout
- **THEN** it SHALL identify that revision and include the required license and notice files

#### Scenario: Dependency inventory
- **WHEN** a backend executable is packaged for a platform
- **THEN** the package SHALL list every Go module linked into it and include each module's license and notice files

### Requirement: Validated package artifact
CI SHALL build and validate the plugin package on pull requests and eligible pushes, run the frontend end-to-end tests against it in a pinned Grafana, and SHALL retain the validated package of eligible pushes as a workflow artifact. This repository SHALL NOT publish the plugin to a catalog or deploy it until a publication workflow exists that depends on the same validation.

#### Scenario: End-to-end failure
- **WHEN** the frontend end-to-end tests fail against the packaged plugin
- **THEN** CI SHALL fail and no package artifact SHALL be retained for that revision

#### Scenario: Validator failure
- **WHEN** plugin package validation fails for a revision
- **THEN** CI SHALL fail and no package artifact SHALL be retained for that revision
