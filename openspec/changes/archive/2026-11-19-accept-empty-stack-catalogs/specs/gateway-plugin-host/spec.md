## MODIFIED Requirements

### Requirement: Catalog route parent check
Before using a catalog route's decrypted values, the Gateway SHALL check that the route carries a parent object whose group, version, kind, namespace and name match the route's group, version, `ModelCatalog`, namespace and `{name}`. A missing or mismatched parent SHALL fail with the internal host error before the body is read, with no access decision, listing, selection or provider work.

#### Scenario: Parent names another catalog
- **WHEN** a catalog route for `team` receives a parent named `other`, or one from another namespace
- **THEN** the response SHALL be the internal host error, and no decrypted value SHALL reach selection or a provider

#### Scenario: Route without a parent
- **WHEN** a catalog route request carries no parent object
- **THEN** the response SHALL be the internal host error

#### Scenario: Parent whose spec does not decode
- **WHEN** an authorized, otherwise valid request carries a matching parent whose spec does not decode or whose required maps are absent or null
- **THEN** each catalog route SHALL return its fixed catalog configuration error (`424`) with no provider requests
- **AND** the Gateway SHALL NOT use partially decoded configuration or decrypted values

#### Scenario: Parent whose identity does not decode
- **WHEN** the parent is not JSON, or its API version, kind, namespace or name cannot be read
- **THEN** the response SHALL be the internal host error
