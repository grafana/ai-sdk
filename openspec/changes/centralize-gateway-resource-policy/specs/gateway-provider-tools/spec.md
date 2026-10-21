# Spec Delta

## MODIFIED Requirements

### Requirement: History-aware deferred result correlation
Output results SHALL match current-response calls or unresolved provider-owned assistant calls from request history. Completed/client-owned historical calls SHALL NOT qualify. Unknown IDs, mismatched names, duplicate calls, results before their matching call, and results after completion SHALL fail safely. Unrelated results in truncated history SHALL NOT seed new pending calls. State SHALL retain only identity/lifecycle information and SHALL be discarded per request.

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
- **WHEN** request history bounded by the shared route request-body bound seeds deferred state
- **THEN** its entries SHALL NOT retain history payloads

### Requirement: Bounded preliminary lifecycle
Streaming SHALL allow multiple correlated previews followed by exactly one final result. Complete calls without results MAY finish for deferred execution, but unfinished previews and post-final results SHALL fail. Cancellation, ordered errors, authoritative finish and bounded cleanup SHALL remain. Pre-call image previews SHALL remain unsupported under WP16 (#110).

#### Scenario: Preview closure
- **WHEN** a call receives two preliminary results and one final result
- **THEN** all events SHALL arrive in order and any later result SHALL fail

#### Scenario: Invalid finish or exceeded bound
- **WHEN** finish arrives during a preview series, output/metadata is invalid, or the upstream response exceeds the shared outbound bound
- **THEN** invalid bytes SHALL NOT be committed and cleanup SHALL remain bounded; output size alone SHALL NOT fail an event

## ADDED Requirements

### Requirement: Tool metadata resource and capture boundaries
Tool output and metadata size SHALL be bounded by gateway-resource-policy rather than aggregate adapter budgets. Returned metadata SHALL NOT authorize telemetry capture.

#### Scenario: Large aggregate tool metadata
- **WHEN** tool metadata namespace objects collectively exceed the former response or frame budget
- **THEN** the output SHALL be delivered complete and metadata SHALL NOT authorize telemetry capture

## REMOVED Requirements

### Requirement: Tool metadata aggregate resource and capture boundaries
**Reason**: Aggregate original-byte, cardinality and complete-envelope budgets are removed.
**Migration**: Use "Tool metadata resource and capture boundaries" and gateway-resource-policy.
