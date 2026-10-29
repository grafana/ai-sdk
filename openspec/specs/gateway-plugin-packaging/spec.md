# gateway-plugin-packaging Specification

## Purpose
Build, license and validate the Gateway app plugin package from the `ai-gateway/` directory using same-revision workspace source for its backend.

## Requirements

### Requirement: Plugin package from the Gateway directory
The `ai-gateway/` directory SHALL be the plugin root. It SHALL contain the plugin metadata, a frontend with an overview page, the app manifest source and its committed generated outputs. The backend SHALL live in `ai-gateway/pkg`, the standard plugin backend root, and SHALL be built from there for Linux amd64 and arm64 at minimum.

#### Scenario: Package contents
- **WHEN** the plugin package is built
- **THEN** it SHALL contain the plugin metadata, frontend bundle, app manifest and backend executables for each supported platform

#### Scenario: Standard plugin tooling finds the backend
- **WHEN** standard plugin tooling tests or builds the backend from the plugin root with its default backend package
- **THEN** it SHALL test and build the Gateway plugin backend, without Gateway-specific path configuration

### Requirement: Generated manifest consistency
The generated manifest data and code SHALL be committed and regenerated from the manifest source with a pinned generator version. Validation SHALL fail when regeneration changes committed output.

#### Scenario: Stale generated manifest
- **WHEN** the manifest source changes without regenerated outputs
- **THEN** validation SHALL fail before packaging

### Requirement: Packaged licensing and source attribution
The package SHALL include the Gateway AGPL license and notice, applicable Apache licensing for linked SDK modules, a per-platform inventory of linked Go modules with their license and notice files, and the licenses of bundled frontend dependencies. Packages built by CI SHALL be attributed to the commit checked out for the workflow run; other builds SHALL be marked as unverified development builds.

#### Scenario: CI package attribution
- **WHEN** a package is built in CI from a checkout whose HEAD equals the workflow run's commit
- **THEN** its `SOURCE_REVISION` SHALL be that full commit SHA, and it SHALL include the required license and notice files

#### Scenario: Unverified build
- **WHEN** a package is built outside CI, or in CI from a checkout whose HEAD differs from the workflow run's commit
- **THEN** its `SOURCE_REVISION` SHALL mark it as an unverified development build

#### Scenario: Dependency inventory
- **WHEN** a backend executable is packaged for a platform
- **THEN** the package SHALL list every Go module linked into it and include each module's license and notice files

### Requirement: Validated package artifact
CI SHALL build and validate the plugin package on pull requests and eligible pushes, run the frontend end-to-end tests against it in a pinned Grafana, and SHALL retain the validated package of eligible pushes as a workflow artifact. Publication to a catalog SHALL follow the `gateway-plugin-publication` capability and SHALL depend on this validation.

#### Scenario: End-to-end failure
- **WHEN** the frontend end-to-end tests fail against the packaged plugin
- **THEN** CI SHALL fail and no package artifact SHALL be retained for that revision

#### Scenario: Validator failure
- **WHEN** plugin package validation fails for a revision
- **THEN** CI SHALL fail and no package artifact SHALL be retained for that revision

### Requirement: Single backend build entrypoint
One plugin build entrypoint SHALL build the Linux amd64 and arm64 backend executables and add the license inventory and `SOURCE_REVISION` to an already built frontend package. Local builds, repository CI and the shared plugin workflow SHALL all use it, so every package has the same contents.

#### Scenario: Shared workflow build
- **WHEN** the shared plugin workflow builds the backend with the Gateway's build entrypoint
- **THEN** the resulting package SHALL contain the same entries as a repository CI build, including the license inventory and `SOURCE_REVISION`

#### Scenario: Frontend not built
- **WHEN** the build entrypoint runs before the frontend package exists
- **THEN** it SHALL fail without producing a partial package
