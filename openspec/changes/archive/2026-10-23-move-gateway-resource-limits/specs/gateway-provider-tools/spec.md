## ADDED Requirements

### Requirement: Tool metadata capture boundary
Tool metadata SHALL NOT be bounded by aggregate original bytes, cardinality or encoded response or frame size. Returned metadata SHALL NOT authorize telemetry capture.

#### Scenario: Large tool metadata
- **WHEN** tool metadata namespace objects together exceed the former response or frame budget
- **THEN** the output SHALL be returned with every namespace unchanged, and metadata SHALL NOT authorize telemetry capture

## MODIFIED Requirements

### Requirement: History-aware deferred result correlation
Output results SHALL match current calls or pending provider-owned history. One matching pending native call re-emission SHALL be accepted in output and accumulated history. Unknown, mismatched, completed, client-owned or repeated duplicates and out-of-order results SHALL fail safely. Truncated unrelated results SHALL NOT seed calls. State SHALL retain only identity/lifecycle data per request.

#### Scenario: Deferred success or error
- **WHEN** a provider call finishes without a result and the next request includes it unresolved in history
- **THEN** unary and streaming success/error results SHALL be accepted without repeating the call

#### Scenario: Invalid historical match
- **WHEN** an output matches only unknown, completed, out-of-order, mismatched or client-owned history
- **THEN** unary output SHALL fail before commitment and streaming SHALL use the bounded safe terminal path

#### Scenario: Native approval call re-emission
- **WHEN** a native approval continuation repeats the matching pending provider-owned call before its result and the consumer appends that returned history
- **THEN** both that response and the next request SHALL accept the coherent single re-emission while rejecting conflicting, completed or repeated duplicate calls

#### Scenario: History accounting
- **WHEN** request history seeds deferred state
- **THEN** its entries SHALL retain only identity and lifecycle, not history payloads

## REMOVED Requirements

### Requirement: Tool metadata aggregate resource and capture boundaries
**Reason**: Tool metadata is no longer bounded by shared aggregate or complete-envelope limits.
**Migration**: The capture rule remains under "Tool metadata capture boundary". Size is bounded by gateway-resource-policy.
