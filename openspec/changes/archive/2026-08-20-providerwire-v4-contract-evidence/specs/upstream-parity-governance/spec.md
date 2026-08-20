## ADDED Requirements

### Requirement: ProviderWire V4 evidence is an exact baseline consumer
The repository SHALL register `test/providerwire-v4` as an exact upstream parity baseline consumer in both baseline validation and coordinated baseline upgrade tooling. Validation SHALL fail when its tracked `ai` or `@ai-sdk/*` dependency is missing from or differs from `test/conformance/upstream.yaml`, and focused tooling tests SHALL detect omission of the workspace from either consumer list.

#### Scenario: ProviderWire evidence dependencies match baseline
- **WHEN** every tracked dependency in `test/providerwire-v4/package.json` matches the registered package version
- **THEN** baseline consumer validation SHALL pass for that workspace

#### Scenario: ProviderWire evidence dependency drifts
- **WHEN** a tracked dependency in `test/providerwire-v4/package.json` differs from the registered package version
- **THEN** baseline validation SHALL fail and identify the workspace, package, declared version, and registered version

#### Scenario: Validation consumer registration is removed
- **WHEN** `test/providerwire-v4` is omitted from the default baseline validation consumer list
- **THEN** a focused baseline-tooling test SHALL fail

#### Scenario: Upgrade consumer registration is removed
- **WHEN** `test/providerwire-v4` is omitted from the coordinated upgrade consumer list
- **THEN** a focused upgrade-tooling test SHALL fail

### Requirement: ProviderWire V4 artifacts participate in baseline upgrades
A registered upstream baseline upgrade that changes a package used by `test/providerwire-v4` SHALL update its exact dependency pins, refresh the explicit relevant source-equivalence closure, regenerate semantic requests and reviewer-facing classification through the public ProviderWire V4 artifact workflow, and review and manually update the canonical typed coverage map, request schemas, semantic-loss decisions, and client-consumption probes when affected. Artifact baseline metadata SHALL derive from or validate against `upstream.yaml` and the workspace manifest. Upgrade completion and required parity CI SHALL require the non-mutating ProviderWire V4 check to pass.

#### Scenario: Upgrade changes request behavior
- **WHEN** an upgraded registered package changes a classified request key, discriminator, serializer transform, presence behavior, or semantic capture
- **THEN** the affected ProviderWire V4 artifacts SHALL change in the same upgrade
- **AND** the difference SHALL be classified before the upgrade is complete

#### Scenario: Upgrade does not change request behavior
- **WHEN** upgraded registered packages produce no ProviderWire V4 contract or evidence difference
- **THEN** deterministic regeneration SHALL leave the committed artifacts unchanged
- **AND** the ProviderWire V4 check SHALL pass

#### Scenario: Upgrade leaves stale ProviderWire evidence
- **WHEN** package pins are updated but committed ProviderWire V4 artifacts no longer reproduce or validate
- **THEN** baseline upgrade verification SHALL fail
