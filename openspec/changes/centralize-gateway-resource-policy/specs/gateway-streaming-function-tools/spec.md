# Spec Delta

## MODIFIED Requirements

### Requirement: Bounded stateless tool correlation and metadata

History-derived state SHALL be bounded by the shared route request-body bound and current-stream state by the outbound response bound, and neither SHALL persist between requests. Tool metadata SHALL retain opaque namespace objects and omitted/empty presence under gateway-provider-metadata; semantic ownership validation SHALL NOT project away unknown fields.

#### Scenario: Bounded stateless tool correlation and metadata
- **WHEN** a continuation carries unresolved history and the new stream supplies a result with an unknown metadata namespace
- **THEN** history and current-stream state SHALL remain request-scoped and nonpersistent, and the namespace SHALL survive without ownership-based projection

### Requirement: Existing streaming bounds and terminal semantics
Tool events SHALL use the existing idle/total deadlines and single cleanup owner, without part-count or frame budgets. Provider error parts SHALL remain ordered and non-terminal without changing tool state. Finish SHALL be the final event followed by immediate clean EOF; no DONE sentinel SHALL be emitted. Write failure SHALL stop without another write.

#### Scenario: Error during input then completion
- **WHEN** a provider error occurs between valid input deltas and the provider later closes the block, emits its call and finish
- **THEN** safe error and subsequent events SHALL remain ordered with no candidate switch

#### Scenario: Cancellation or oversized result
- **WHEN** cancellation occurs with open input, or a valid tool result encodes larger than the former complete-frame budget
- **THEN** cancellation SHALL cancel provider work with bounded owned cleanup and no partial event, while the large valid result SHALL be written complete and in order
