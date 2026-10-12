# gateway-workspace-container Specification

## Purpose

Define same-revision workspace source selection and dependency/license evidence for Gateway container images.

## Requirements

### Requirement: Gateway images build checked-out workspace source

Local builds and eligible main SHA and Gateway-version-tag image builds SHALL use one explicit repository-root context, Gateway Dockerfile and checked-in `go.gateway.work` selection. The build SHALL use the Gateway command plus selected SDK/provider/middleware source from that build's working tree, rather than downloading older internal implementations from committed merged pins.

#### Scenario: Coordinated source with older internal pins
- **WHEN** a Gateway command uses a local SDK/provider change at a selected revision but its older merged declared pin lacks that behavior
- **THEN** the built image SHALL exercise the changed local implementation even if standalone Gateway compilation against that pin fails
- **AND** unchanged readonly module manifests SHALL remain unchanged by the build

#### Scenario: Main and versioned images
- **WHEN** a main push or an `ai-gateway/vX.Y.Z` tag push is eligible for image publication
- **THEN** each SHALL use the same workspace-source recipe at the selected push SHA
- **AND** the tag SHALL determine its checkout revision and image tag, not a different dependency mode

#### Scenario: Local recipe
- **WHEN** the repository's local Gateway image command executes with working-tree changes
- **THEN** it SHALL select the same Dockerfile, root context and checked-in workspace dependency mode as validation and publication, and build from those local source files
- **AND** its OCI revision and local-module inventory SHALL identify the build as unverified development source, not as the unchanged HEAD commit

#### Scenario: Published image source
- **WHEN** a main or Gateway-tag image build runs at the selected push SHA
- **THEN** validation and publication SHALL verify a clean checkout with HEAD equal to that push SHA before building and attribute their OCI revision and local-module inventory to it
- **AND** a failed cleanliness or revision check SHALL block publication

### Requirement: Image source verification and development attribution

Image validation and publication SHALL build from a fresh clean checkout at `GITHUB_SHA`, verified with HEAD equality and standard Git cleanliness checks, and SHALL attribute the resulting image and local-module inventory to that SHA. Local builds MAY include uncommitted changes and SHALL use explicit unverified-development attribution in both OCI revision and local-module inventory rather than claim those bytes match HEAD. The root `go.work` SHALL continue to exclude Gateway.

#### Scenario: Image source verification and development attribution
- **WHEN** a local build uses dirty source and a publication build has HEAD different from GITHUB_SHA
- **THEN** local OCI revision/inventory SHALL declare unverified development and publication SHALL fail its clean revision check without changing root workspace exclusion

### Requirement: Pinned readonly Docker build inputs

Docker builds SHALL use readonly manifests and checksum-verified pinned external dependencies and pinned build/base images; SHALL NOT run floating dependency updates or use ambient/out-of-repository workspace substitutions.

#### Scenario: Pinned readonly Docker build inputs
- **WHEN** the Gateway Docker build resolves dependencies
- **THEN** it SHALL use readonly manifests, verified pinned dependencies and pinned images without floating updates or ambient workspace substitution

### Requirement: Image inputs and runtime remain bounded

The Gateway Dockerfile-specific root-context ignore policy SHALL include exactly the required workspace manifests/checksums, referenced local module manifests/source, command assets (including embedded files) and applicable license/notice inputs.

#### Scenario: Root context contains local modules
- **WHEN** a context is prepared for Docker
- **THEN** the referenced workspace modules, command embeds and licensing inputs SHALL be present
- **AND** sentinel secrets, local config, Git metadata and development caches SHALL not be sent to the builder or copied into runtime layers

#### Scenario: Runtime image is launched
- **WHEN** the native image runs with the existing mounted configuration and runtime credentials
- **THEN** it SHALL retain UID/GID `10001:10001`, readiness behavior and the current runtime settings
- **AND** credentials SHALL NOT have been supplied as image build arguments

### Requirement: Gateway root-context exclusions and runtime layers

The Gateway root-context ignore policy SHALL exclude Git metadata, credentials, local configuration, node_modules and unrelated development/build caches or artifacts; the existing Gateway-directory `.dockerignore` SHALL NOT stand in for root-context filtering, and Gateway-specific rules SHALL NOT govern other Dockerfiles using the repository-root context. Final image layers SHALL contain only the built binary and necessary runtime/license artifacts.

#### Scenario: Gateway root-context exclusions and runtime layers
- **WHEN** the root context contains sentinel credentials and unrelated caches
- **THEN** Gateway-specific filtering SHALL exclude them without substituting the directory ignore file or governing other Dockerfiles, and final layers SHALL contain only necessary binary/runtime/license artifacts

### Requirement: Preserved Gateway runtime image behavior

Gateway SHALL retain nonroot execution, configuration behavior and amd64/arm64 support.

#### Scenario: Preserved Gateway runtime image behavior
- **WHEN** an amd64 or arm64 image starts with existing mounted configuration
- **THEN** nonroot execution and existing configuration behavior SHALL remain supported

### Requirement: Image dependency and license evidence matches the build

The image SHALL inventory deduplicated modules used by the actual Gateway command's target-platform package dependency closure under the same explicit workspace and readonly resolution as compilation. Each used local workspace module SHALL be attributed to the verified push SHA for published images or the unverified-development source for local builds, even when `.Version` is empty; each used external module SHALL identify its resolved version and verified checksum.

#### Scenario: Workspace modules lack release versions
- **WHEN** `go list -deps` reports used local modules without `.Version`
- **THEN** the inventory SHALL include them at the verified repository SHA for published images or as unverified development source for local images, and package their applicable first-party license material

#### Scenario: Target-specific external dependencies
- **WHEN** amd64 and arm64 Gateway command closures are determined
- **THEN** each platform's inventory SHALL correspond to its own compiled command closure and name resolved external versions/checksums and applicable dependency notices
- **AND** unused workspace modules SHALL NOT be misreported as command dependencies

### Requirement: Packaged image licenses and consistent source attribution

The image SHALL package applicable root Apache SDK licensing for nested local SDK modules, Gateway AGPL license and notice, and existing discoverable dependency licensing. OCI revision and packaged inventory SHALL consistently distinguish verified published source from unverified local source; no new SBOM platform is required.

#### Scenario: Packaged image licenses and consistent source attribution
- **WHEN** an image uses nested local SDK modules and external dependencies
- **THEN** it SHALL package applicable Apache, AGPL, notice and discoverable dependency licensing with revision/inventory attribution matching verified or development source, without requiring a new SBOM platform
