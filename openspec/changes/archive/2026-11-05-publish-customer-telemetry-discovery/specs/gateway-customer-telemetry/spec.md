## ADDED Requirements

### Requirement: Ordered discovery publication

Accepted discovery and policy changes SHALL reach delivery authority in local acceptance order, including background changes without another model request. Requests SHALL NOT restore authority by publishing retained snapshots. Revisions SHALL remain opaque and independent signal decisions SHALL be preserved.

#### Scenario: Background disablement

- **WHEN** policy refresh disables a signal with current and retiring exporters and no subsequent request
- **THEN** both generations reject new export admissions after publication completes
- **AND** independently permitted signals remain available

#### Scenario: Late independent result

- **WHEN** a destination lookup completes after a disabled policy was accepted
- **THEN** publication combines that destination with the current disabled policy
- **AND** neither older request snapshots nor timed-out lookup results restore permission

### Requirement: Current-state delivery acquisition

Delivery acquisition SHALL use current validated metadata and return an immutable recording policy with its typed lease. Discovery and construction SHALL remain lazy and bounded. Acquisition SHALL recover from reclaimed pool capacity or idle bindings using current metadata without requiring new metadata revisions.

#### Scenario: Replacement capacity becomes available

- **WHEN** replacement initially fails for lack of pool capacity and retired resources later finish
- **THEN** the next acquisition can use the current destination without another metadata revision

#### Scenario: Selection loses authority

- **WHEN** a selected binding is revoked or replaced before acquisition completes
- **THEN** acquisition omits recording rather than republishing its previous policy

### Requirement: Retiring authority cannot outlive shortened validity

A permitted publication with a shorter validity SHALL shorten affected retiring resources as well as the current resource, including when replacement cannot fit. Refreshing a successor SHALL NOT extend an already retired resource's validity or reactivate a revoked gate.

#### Scenario: Retiring lease outlives successor

- **WHEN** a replacement has a shorter validity than an active retired lease and refresh cannot finish
- **THEN** both resources reject new exports by the shorter deadline
- **AND** observed expiry cancels outstanding operations without waiting for another request

### Requirement: Bounded composed discovery ownership

Discovery eviction SHALL revoke and forget its delivery authority. Background publication SHALL NOT count as request activity. Composed shutdown SHALL stop publication and start bounded pool draining without waiting for non-cooperative resolvers. Retained work SHALL remain accounted until actual completion.

#### Scenario: Idle stack still owns a lease

- **WHEN** the cache evicts a stack after idle retention while a lease remains held
- **THEN** its exports are revoked and publication metadata is removed
- **AND** re-admitting the stack cannot restore the old lease

#### Scenario: Resolver blocks shutdown

- **WHEN** shutdown starts while a resolver ignores cancellation
- **THEN** pools still begin draining and new acquisition is refused
- **AND** shutdown returns within its bound without extending deadlines on repeated calls
