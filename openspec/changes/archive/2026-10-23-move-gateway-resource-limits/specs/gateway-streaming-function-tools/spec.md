## MODIFIED Requirements

### Requirement: Bounded stateless tool correlation and metadata

History-derived state SHALL be bounded by the route request-body bound, current-stream state by the stream's duration limits, and neither SHALL persist between requests. Tool metadata SHALL retain opaque namespace objects and omitted/empty presence under gateway-provider-metadata; semantic ownership validation SHALL NOT project away unknown fields.

#### Scenario: Bounded stateless tool correlation and metadata
- **WHEN** a continuation carries unresolved history and the new stream supplies a result with an unknown metadata namespace
- **THEN** history and current-stream state SHALL remain request-scoped and nonpersistent, and the namespace SHALL survive without ownership-based projection

### Requirement: Existing streaming bounds and terminal semantics
Tool events SHALL use the existing idle/total deadlines and single cleanup owner, without a part count or frame budget. Provider error parts SHALL remain ordered and non-terminal without changing tool state. Finish SHALL be the final event followed by immediate clean EOF; no DONE sentinel SHALL be emitted. Write failure SHALL stop without another write.

#### Scenario: Error during input then completion
- **WHEN** a provider error occurs between valid input deltas and the provider later closes the block, emits its call and finish
- **THEN** safe error and subsequent events SHALL remain ordered with no candidate switch

#### Scenario: Cancellation or oversized result
- **WHEN** cancellation occurs with open input
- **THEN** provider work SHALL be canceled, owned cleanup SHALL remain bounded and no partial event SHALL be written
- **WHEN** a valid tool input delta, call or result encodes to a frame above the former 1 MiB limit, or a stream opens more tool calls than the former part count
- **THEN** each complete event SHALL be written in order and the stream SHALL continue
