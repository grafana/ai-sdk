# Spec Delta

## ADDED Requirements

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
