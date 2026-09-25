## ADDED Requirements

### Requirement: Gateway images build checked-out workspace source
Local builds and eligible main SHA and Gateway-version-tag image builds SHALL use one explicit repository-root context, Gateway Dockerfile and checked-in `go.gateway.work` selection. The build SHALL use the Gateway command plus selected SDK/provider/middleware source from the same checked-out revision, rather than downloading older internal implementations from committed merged pins. All SHA-attributed local and CI image builds SHALL verify before building that Docker-included inputs and the Dockerfile/context-selection/build policy files are tracked and unchanged from the selected commit, rejecting modified/deleted tracked or admitted untracked inputs (including ignored files). Local builds SHALL require these clean inputs at HEAD; validation and publication SHALL additionally verify HEAD equals `GITHUB_SHA`. OCI revision and local-module inventory SHALL carry only that verified commit SHA. The root `go.work` SHALL continue to exclude Gateway. Docker builds SHALL use readonly manifests and checksum-verified pinned external dependencies and pinned build/base images; SHALL NOT run floating dependency updates or use ambient/out-of-repository workspace substitutions.

#### Scenario: Coordinated source with older internal pins
- **WHEN** a Gateway command uses a local SDK/provider change at a selected revision but its older merged declared pin lacks that behavior
- **THEN** the built image SHALL exercise the changed local implementation even if standalone Gateway compilation against that pin fails
- **AND** unchanged readonly module manifests SHALL remain unchanged by the build

#### Scenario: Main and versioned images
- **WHEN** a main push or an `ai-gateway/vX.Y.Z` tag push is eligible for image publication
- **THEN** each SHALL use the same workspace-source recipe at the selected push SHA
- **AND** the tag SHALL determine its checkout revision and image tag, not a different dependency mode

#### Scenario: Local recipe
- **WHEN** the repository's local Gateway image command executes with clean Docker-relevant inputs
- **THEN** it SHALL select the same Dockerfile, root context and checked-in workspace source as validation and publication
- **AND** it SHALL use the verified HEAD SHA for both OCI revision and local-module inventory

#### Scenario: Matching HEAD with dirty build input
- **WHEN** HEAD matches the selected SHA but a Docker-relevant tracked input has changed, or an admitted untracked input exists (including Git-ignored files)
- **THEN** local, validation and publication image builds SHALL fail before building rather than attribute different bytes to the selected SHA
- **AND** excluded ambient files SHALL remain outside the build context

### Requirement: Image inputs and runtime remain bounded
The root Docker context SHALL include exactly the required workspace manifests/checksums, referenced local module manifests/source, command assets (including embedded files) and applicable license/notice inputs. It SHALL exclude Git metadata, credentials, local configuration, node_modules and unrelated development/build caches or artifacts; the Gateway-only `.dockerignore` SHALL NOT stand in for root-context filtering. Final image layers SHALL contain only the built binary and necessary runtime/license artifacts. Gateway SHALL retain nonroot execution, configuration behavior and amd64/arm64 support.

#### Scenario: Root context contains local modules
- **WHEN** a context is prepared for Docker
- **THEN** the referenced workspace modules, command embeds and licensing inputs SHALL be present
- **AND** sentinel secrets, local config, Git metadata and development caches SHALL not be sent to the builder or copied into runtime layers

#### Scenario: Runtime image is launched
- **WHEN** the native image runs with the existing mounted configuration and runtime credentials
- **THEN** it SHALL retain UID/GID `10001:10001`, readiness behavior and the current runtime settings
- **AND** credentials SHALL NOT have been supplied as image build arguments

### Requirement: Image dependency and license evidence matches the build
The image SHALL inventory deduplicated modules used by the actual Gateway command's target-platform package dependency closure under the same explicit workspace and readonly resolution as compilation. Each used local workspace module SHALL be attributed to the verified clean source SHA even when `.Version` is empty; each used external module SHALL identify its resolved version and verified checksum. It SHALL package applicable root Apache SDK licensing for nested local SDK modules, Gateway AGPL license and notice, and existing discoverable dependency licensing. OCI revision and packaged inventory SHALL identify the source built; no new SBOM platform is required.

#### Scenario: Workspace modules lack release versions
- **WHEN** `go list -deps` reports used local modules without `.Version`
- **THEN** the inventory SHALL include them at the selected repository SHA and package their applicable first-party license material

#### Scenario: Target-specific external dependencies
- **WHEN** amd64 and arm64 Gateway command closures are determined
- **THEN** each platform's inventory SHALL correspond to its own compiled command closure and name resolved external versions/checksums and applicable dependency notices
- **AND** unused workspace modules SHALL NOT be misreported as command dependencies
