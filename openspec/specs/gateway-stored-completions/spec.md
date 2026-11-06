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

### Requirement: Bounded stable listing

Stored completion listing SHALL filter by verified owner, model and metadata,
order by creation time with ID as a deterministic tie-breaker, and support
ascending/descending limit/after pagination. The backend SHALL scan resource
pages while retaining at most limit plus one complete records. An unbound
backend SHALL be rejected before billable model I/O.

#### Scenario: Duplicate timestamps and sparse metadata
- **WHEN** records share timestamps and a metadata filter is supplied
- **THEN** ID ordering is deterministic and absent metadata does not match an empty string
- **AND** the after cursor is resolved only within the caller's scope.

### Requirement: Opt-in lifecycle and streaming persistence

The Chat adapter SHALL persist only requests with store true, expose scoped
retrieve/list/update-metadata/delete/messages routes, and retain original input
history. Native provider storage SHALL remain disabled. Successful unary output
and streaming DONE SHALL follow durable creation, with aggregate usage retained
once for multiple choices. A failed persistence SHALL return an explicit error
and SHALL NOT emit DONE.

#### Scenario: Streaming storage failure
- **WHEN** a streamed stored completion cannot be durably created
- **THEN** the client receives a stream error without DONE
- **AND** the handler does not announce stored success.

#### Scenario: Original multimodal history
- **WHEN** stored messages are requested after a multimodal or legacy function request
- **THEN** original names, roles and content parts remain available with stable pagination.

### Requirement: Configurable retention and bounded records

Records SHALL be limited to 512 KiB of logical stored content. Streaming quota
checks SHALL NOT depend on fragmentation, repeated metadata or obfuscation.
Retention SHALL default to 30 days and be configurable from one minute through
365 days. Public protocol roles SHALL NOT grant raw-kind access.

#### Scenario: Oversized conversation
- **WHEN** a completion and its original messages exceed the storage bound
- **THEN** storage returns an error without creating or replacing a record
- **AND** input history and metadata already exceeding the quota fail with 413 before model I/O.

#### Scenario: Unprovisioned backend
- **WHEN** the private kind is missing or scoped list access is denied
- **THEN** a bounded readiness probe fails with 503 before model I/O.
