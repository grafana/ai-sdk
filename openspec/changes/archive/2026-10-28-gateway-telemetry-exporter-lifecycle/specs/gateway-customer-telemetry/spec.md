# Spec Delta

## ADDED Requirements

### Requirement: Isolated delivery resources

Customer delivery resources SHALL be isolated by authorized stack, signal, destination and credential version. Shared construction SHALL NOT retain caller authentication context. Policy revision changes alone SHALL NOT rebuild unchanged transport configuration. Invalid or disallowed state SHALL NOT acquire a resource.

#### Scenario: Two customers export concurrently

- **WHEN** customers A and B acquire resources for distinct signal identities
- **THEN** their requests use only their own configured endpoints and authentication
- **AND** construction failure for A does not prevent B from acquiring

#### Scenario: Concurrent acquisition

- **WHEN** multiple requests acquire the same current binding
- **THEN** only one resource construction occurs
- **AND** cancellation of one waiting request does not cancel another waiter

### Requirement: Versioned recording leases

A recording lease SHALL retain its immutable delivery configuration until release or revocation. Permitted rotation SHALL direct new acquisitions to the replacement and retire the old resource after its last lease. Replaced bindings SHALL NOT acquire or restore retired resources. Release SHALL relinquish recording ownership and be idempotent.

#### Scenario: Credential rotation while streaming

- **WHEN** a credential version changes while an old recording lease is active
- **THEN** new leases use the new credential version
- **AND** the old lease can finish with its original configuration while still valid
- **AND** the old resource shuts down exactly once after release

### Requirement: Delivery revocation and expiry

Observed disablement, removal, invalid state or denial SHALL revoke affected current and retiring delivery resources. Expiry SHALL prevent new network export. Queued exports SHALL pass through the same authority gate; revocation SHALL cancel in-flight operations without claiming to retract already-delivered data.

#### Scenario: Disabled signal has queued records

- **WHEN** the owner publishes a disabled policy before queued records are sent
- **THEN** no new network export starts for that signal, including during shutdown
- **AND** independently allowed signals remain usable

#### Scenario: Export redirected to another destination

- **WHEN** an export response redirects or a client attempts a different configured target
- **THEN** the gated client does not forward the authenticated request

### Requirement: Bounded delivery ownership

Delivery ownership SHALL bound retained resources, construction waiting, failed-construction retries, export operations, idle lifetime and shutdown. Pending and retiring work SHALL count toward capacity until actual completion. Failures SHALL expose fixed diagnostics without raw credentials or endpoint details.

#### Scenario: Construction ignores cancellation

- **WHEN** construction exceeds its deadline but remains running
- **THEN** waiters return and the outstanding construction remains charged to capacity
- **AND** a late resource is cleaned up rather than published

#### Scenario: Close with active leases

- **WHEN** shutdown begins during recording
- **THEN** new acquisition stops and active leases have a bounded drain period
- **AND** remaining gates are revoked at the shutdown deadline
- **AND** close honors the caller deadline even if resource cleanup ignores cancellation

#### Scenario: Cleanup cannot establish teardown

- **WHEN** a resource shutdown callback returns an error
- **THEN** its export authority is revoked and its capacity remains reserved while the pool is open
- **AND** closure reports a fixed cleanup failure without raw diagnostics

### Requirement: Supported delivery lifecycle evidence

Lifecycle introduction SHALL preserve existing production inference and operator telemetry. Transport compatibility SHALL be demonstrated through a supported exporter API and local receivers; unsupported SDK cancellation or queue controls SHALL NOT be simulated as production capability.

#### Scenario: Local exporter verification

- **WHEN** the pinned OTLP HTTP exporter runs through the pool's supported client injection
- **THEN** local receivers verify serialized requests, tenant isolation, rotation and revocation
- **AND** this evidence is not presented as live Cloud authorization or ingestion proof
