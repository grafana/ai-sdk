# Gateway stored completions

## Purpose

Provide durable storage for explicitly requested Chat completions while keeping
conversation data scoped to the verified caller.

## Requirements

### Requirement: Durable scoped storage

Stored completions SHALL use the host app-sdk resource backend, with owner scope
derived from verified namespace and identity. Public roles SHALL have no access
to the underlying kind. Records SHALL omit provider options, credentials,
transport headers and execution diagnostics. Creation SHALL be opt-in and bounded.

#### Scenario: Different caller or namespace
- **WHEN** another identity or namespace requests an existing completion ID
- **THEN** retrieval, metadata mutation and deletion return not found
- **AND** listing does not include that record.

#### Scenario: Backend restart
- **WHEN** a new store instance connects to the same resource backend
- **THEN** previously stored unexpired completions remain retrievable.

### Requirement: Expiration and atomic mutation

Expiry SHALL be enforced on every read and list regardless of cleanup timing.
The backend SHALL provide an expired-resource sweep and conditional updates and
deletes so concurrent mutations cannot resurrect removed records.

#### Scenario: Expired record
- **WHEN** retention has elapsed
- **THEN** the record is unavailable immediately
- **AND** cleanup removes its durable resource.
