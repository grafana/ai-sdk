## MODIFIED Requirements

### Requirement: Post-publication proxy verification

After a library GitHub Release is published, a separate read-only workflow SHALL
resolve the new version through the public Go proxy and checksum database in a fresh
cache, with no replacements, build a consumer of its public packages and run its
deterministic consumer scenarios. It SHALL first wait, for a bounded time, until the
version and its first-party requirements resolve, and SHALL be rerunnable.
A failure SHALL be reported for a fix-forward release and SHALL NOT move or delete tags.

#### Scenario: Published library verified

- **WHEN** a library tag is created
- **THEN** an external consumer of that version builds and passes its consumer scenarios through the public proxy

#### Scenario: Published library fails as a consumer

- **WHEN** the new version fails to resolve, build or pass its consumer scenarios through the public proxy
- **THEN** the verification workflow reports the failure and the tag is left unchanged

#### Scenario: Proxy has cached a missing version

- **WHEN** the proxy still answers that a just-tagged version or one of its first-party requirements is unknown
- **THEN** verification waits and retries until it resolves or the bounded wait ends

### Requirement: Explicit staged library activation

Library automation SHALL require explicit activation on canonical main pushes and verified protection settings. Root bootstrap SHALL select only the retained initial root candidate. Library mode SHALL exclude Gateway and preserve independent component outcomes. An absent or unknown mode SHALL prevent writes. Gateway delivery SHALL NOT gate library activation. When main advanced since the run started, preparation SHALL be skipped without failing the run.

#### Scenario: Root bootstrap

- **WHEN** root mode is selected
- **THEN** only the retained bootstrap root candidate can publish
- **AND** no additional release PR is prepared

#### Scenario: Library progression

- **WHEN** library mode is selected and one component remains blocked
- **THEN** other independently valid libraries can publish and prepare releases
- **AND** Gateway receives no writes

#### Scenario: Missing protections

- **WHEN** strict source/readiness checks or immutable tag protections cannot be verified
- **THEN** activation refuses all release writes

#### Scenario: Main advanced during the run

- **WHEN** another commit lands on main while a release run waits for its source checks
- **THEN** the run publishes its frozen inventory, skips preparation with a notice and succeeds unless a candidate is blocked
- **AND** the run for the newer push prepares the release pull request
