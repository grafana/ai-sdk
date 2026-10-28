# gateway-customer-telemetry Specification

## Purpose

Define the Gateway's customer telemetry resolution boundary so destinations, recording policy, and ingestion authority can evolve independently without mixing customer stacks or operator telemetry. This initial contract does not activate delivery.

## Requirements

### Requirement: Authorized stack attribution
Customer telemetry resolution SHALL be scoped to the stack established by Gateway authorization. Missing or refused attribution SHALL yield no customer recording plan. Resolved state belonging to a different stack SHALL be rejected without substituting an operator or default stack.

#### Scenario: Destination belongs to another customer
- **WHEN** a resolution result for authorized stack A identifies stack B
- **THEN** validation rejects that result without a fallback destination

#### Scenario: No authorized customer stack
- **WHEN** attribution is empty or access is refused
- **THEN** no customer recording plan can be validated

### Requirement: Independent signal destinations
Resolution SHALL represent generation, trace, metric, and log destinations independently, including ingestion identities and credential references. Unavailable signals SHALL be explicit. A destination SHALL NOT imply product eligibility or authorize recording. Invalid destinations SHALL be rejected before transport use.

#### Scenario: Distinct ingestion identities
- **WHEN** a stack has different generation, trace, metric, and log ingestion identities
- **THEN** resolution preserves all four without substituting stack or other signal identifiers

#### Scenario: Partial availability
- **WHEN** a valid metric destination exists and generation delivery is unavailable
- **THEN** metric destination validation succeeds independently while generation delivery remains unavailable

#### Scenario: Unsafe destination
- **WHEN** a destination lacks its required identity or contains a non-HTTPS URL, URL credentials, query, or fragment
- **THEN** validation rejects it without echoing the rejected value in diagnostics

### Requirement: Separate ingestion authority
Destination and policy snapshots SHALL contain no ingestion secrets. Credential references SHALL identify a separate authority source, independently versioned from recording policy. Caller authentication and provider keys SHALL NOT be treated as ingestion credentials. Diagnostics SHALL omit raw credentials and destination values.

#### Scenario: Credential rotation
- **WHEN** an ingestion credential revision changes for the same destination and policy
- **THEN** the credential change can be distinguished without changing the policy revision
- **AND** destination and policy snapshots contain no credential material

### Requirement: Conservative default recording policy
The default policy SHALL permit only metadata-only generations and traces and a fixed, documented model-metric profile for independently eligible signals. Customer log recording SHALL default to disabled. Unknown eligibility SHALL NOT grant access. Preferences SHALL NOT override product ineligibility or mandatory service restrictions.

#### Scenario: Eligible stack with no custom preference
- **WHEN** eligibility is authoritative and the optional customer preference is absent
- **THEN** the policy selects documented defaults for eligible signals
- **AND** prompt, output, reasoning, tool payloads, raw errors, arbitrary attributes, and raw request or response data remain excluded

#### Scenario: Product eligibility differs by signal
- **WHEN** generations are ineligible but metrics are eligible
- **THEN** the policy disables generations without disabling independently eligible model metrics

#### Scenario: No customer logging producer
- **WHEN** a log destination is available under the default policy
- **THEN** customer log recording remains disabled and operator logs are not forwarded

### Requirement: Absence and policy failure are distinct
Policy evaluation SHALL distinguish absent optional preferences, explicit disablement, and lookup failure. A failure SHALL NOT select absent-preference defaults or discard a known disablement. Policy validity SHALL be explicit; expired or invalid policy SHALL NOT authorize new recording.

#### Scenario: Failed read after disablement
- **WHEN** a customer preference was disabled and its refresh fails
- **THEN** the result does not restore default recording

#### Scenario: First read fails
- **WHEN** no valid policy is available and a policy read fails
- **THEN** no customer recording plan is authorized

#### Scenario: Policy expires
- **WHEN** the current time reaches a policy's validity deadline
- **THEN** that policy cannot authorize new recording

### Requirement: Stable recording snapshots
Validated destination and policy snapshots SHALL preserve stack attribution and values when caller-owned inputs change. Policy, destination, and credential changes SHALL be independently distinguishable. Default profile changes SHALL require explicit compatibility review; upgrades SHALL NOT silently broaden content capture or metric dimensions.

#### Scenario: Input mutation
- **WHEN** a caller changes an input collection after snapshot construction
- **THEN** the validated snapshot retains its original values

#### Scenario: Policy-only update
- **WHEN** recording is disabled without changing a destination or credential reference
- **THEN** the policy revision changes independently of those destination and credential references

### Requirement: Contract introduction does not activate delivery
Introducing these contracts SHALL NOT start discovery, create customer exporters, change existing operator recording, or change inference requests, responses, or streams. The package SHALL be testable without Cloud credentials, network access, or a customer configuration resource.

#### Scenario: Existing Gateway startup
- **WHEN** the Gateway starts after the contract package is introduced
- **THEN** startup performs no new customer discovery or delivery
- **AND** existing operator telemetry and inference behavior remain unchanged

### Requirement: Demand-driven authorized resolution

The customer telemetry cache SHALL resolve only an explicitly requested authorized stack. Construction SHALL NOT enumerate customers or invoke resolvers. Invalid attribution and already-canceled requests SHALL perform no lookup. Cold requests SHALL obtain usable matching snapshots without requiring prior enrollment.

#### Scenario: Previously unseen eligible stack

- **WHEN** a caller requests telemetry resolution for a valid stack absent from the cache
- **THEN** the cache resolves its destination and policy snapshots within its lookup budget
- **AND** usable results become available to that request without a restart or configuration edit

#### Scenario: No authorized request

- **WHEN** the cache is constructed or receives empty attribution or an already-canceled context
- **THEN** it performs no customer lookup

### Requirement: Independent deduplicated lookups

Concurrent requests SHALL share at most one outstanding resolver invocation per stack and component, with destination and policy components independent. A canceled waiter SHALL stop waiting without canceling other waiters' work. One stack's resolution SHALL NOT serialize unrelated stacks behind its network wait.

#### Scenario: Concurrent first requests

- **WHEN** several requests for stack A arrive while its destination and policy lookups are pending
- **THEN** each component is invoked once
- **AND** canceling one waiter does not prevent another waiter receiving the results

#### Scenario: Another stack resolves while A waits

- **WHEN** A's destination resolver is blocked and B requests resolution
- **THEN** B can resolve independently within the resource limits
- **AND** neither result changes the other's attribution

### Requirement: Bounded metadata waiting

Metadata waiting SHALL have a finite budget and honor each request's cancellation. Cold requests with insufficient remaining deadline for the configured lookup budget and inference headroom SHALL skip telemetry lookup. Cache outcomes SHALL describe omitted telemetry without invoking, rejecting, or replaying inference.

#### Scenario: Insufficient request budget

- **WHEN** no usable snapshots exist and the request's remaining time is below the configured lookup budget plus headroom
- **THEN** the cache returns a bounded budget outcome without starting a lookup

#### Scenario: Resolver timeout

- **WHEN** a lookup reaches its budget
- **THEN** affected waiters return a timeout outcome unless canceled earlier
- **AND** a late result from that timed-out attempt cannot replace current state

### Requirement: Independent freshness and observed revocation

Destination and policy validity SHALL be tracked independently. Active entries SHALL schedule refresh before expiry when lifetime permits minimum lookup spacing. Expiry SHALL stop new recording even if refresh cannot finish in time. Transient failure SHALL NOT extend validity. Observed disablement, destination removal, or authorization denial SHALL stop new affected selections without waiting for previous expiry.

#### Scenario: Transient refresh failure

- **WHEN** an enabled snapshot's refresh fails transiently
- **THEN** it remains usable only until its original validity deadline
- **AND** no expired snapshot authorizes new recording

#### Scenario: Disablement followed by read failure

- **WHEN** a refresh returns a disabled policy and a later policy read fails
- **THEN** subsequent selections remain disabled rather than restoring the previous enabled policy or defaults

#### Scenario: A signal disappears

- **WHEN** destination refresh succeeds with a previously available signal absent
- **THEN** subsequent selections omit that signal immediately
- **AND** other valid signal destinations remain independently available

### Requirement: Bounded negative caching and diagnostics

Unavailable or disallowed results and failed lookups SHALL have bounded retry suppression. Later enablement SHALL become visible without restart. Outcomes SHALL distinguish lookup denial, invalid data, transient failure, timeout, and valid non-recording state without exposing raw errors, credentials, endpoints, or customer identifiers as diagnostic labels.

#### Scenario: Product becomes eligible

- **WHEN** a valid non-recording policy is cached and the source later permits recording
- **THEN** the cache rechecks after the configured negative refresh interval and can use the new policy

#### Scenario: Failure burst

- **WHEN** many requests arrive during retry suppression after a failed lookup
- **THEN** they do not each invoke the failing resolver
- **AND** they receive fixed outcome classifications without raw resolver diagnostics

#### Scenario: Repeated short-lived metadata

- **WHEN** a resolver repeatedly returns the same nearly expired snapshot
- **THEN** attempts respect minimum retry spacing rather than forming a tight loop
- **AND** expiry stops new recording even if retry suppression remains active

### Requirement: Bounded cache ownership and shutdown

The cache SHALL bound retained entries and outstanding resolver calls, reclaim idle entries, and stop accepting work when closed. Capacity exhaustion SHALL omit affected telemetry. Close SHALL cancel owned work and honor its supplied deadline. Late completions SHALL NOT resurrect evicted entries or publish after closure.

#### Scenario: Capacity and reclamation

- **WHEN** the cache reaches capacity and another stack requests resolution
- **THEN** it reclaims eligible idle entries or returns a capacity outcome
- **AND** later requests can admit new stacks after capacity is reclaimed

#### Scenario: Resolver ignores cancellation

- **WHEN** a resolver does not return after cancellation
- **THEN** waiters and close remain bounded
- **AND** its outstanding work remains charged to capacity rather than allowing unbounded replacement calls

#### Scenario: Close races with completion

- **WHEN** close begins while requests and resolver completions are in flight
- **THEN** new work is refused, waiters are released, and late results are ignored

### Requirement: Cache introduction preserves runtime activation boundaries

Introducing the cache SHALL NOT connect it to production routes, construct exporters, or change operator telemetry or inference protocols. Its behavior SHALL be verifiable with deterministic clocks and local fake resolvers without Cloud access.

#### Scenario: Existing Gateway startup with cache code present

- **WHEN** the production Gateway starts with this package compiled
- **THEN** no customer lookup or exporter is started by the new cache code

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
