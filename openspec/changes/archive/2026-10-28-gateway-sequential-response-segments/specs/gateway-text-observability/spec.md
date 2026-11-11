## ADDED Requirements

### Requirement: Agent Observability first-segment limitation
Until the `middleware/agentobservability` recorder change in #582 lands, the Agent Observability generation SHALL be finalized once at a stream's first finish, or at a logical finalization condition when no finish arrives first. Usage, finish reason and timing from parts after that first finish SHALL be excluded from the generation, and every part SHALL still pass through unchanged.

#### Scenario: Multi-segment stream exports the first segment
- **WHEN** a stream carries two segments that each end in a finish with different usage
- **THEN** logs and metrics SHALL observe both segments
- **AND** the Agent Observability generation SHALL carry the first finish's usage and finish reason

## MODIFIED Requirements

### Requirement: Logical text lifecycle semantics

Unary observation SHALL start immediately before the shared model invocation and finalize once after its result or pre-result error. Streaming observation SHALL start immediately before stream setup. Logs and metrics SHALL finalize once after normal channel close, provider error observation, premature close, downstream cancellation, or timeout as represented at the model boundary. A stream with several response segments SHALL remain one logical call.

#### Scenario: Stream error precedes later finish
- **WHEN** a stream emits usage, a provider error part, later text, and finish before closing
- **THEN** every part SHALL pass through unchanged and in order
- **AND** logical observers SHALL finalize once with the established error classification and strongest usage
- **AND** middleware SHALL not terminate the stream or trigger fallback

#### Scenario: Usage is split across stream parts
- **WHEN** different stream parts report independently stronger usage counters
- **THEN** the terminal logical log and metrics SHALL preserve the strongest normalized counters across every part
- **AND** the Agent Observability generation SHALL preserve the strongest normalized counters across the parts up to and including the stream's first finish, which covers every part of a stream with one response segment

#### Scenario: Stream with several response segments
- **WHEN** a stream emits two response segments, each ending with a finish that reports its own usage and finish reason, and then closes
- **THEN** every part SHALL pass through unchanged and in order, and the request SHALL produce one logical log record, one metric observation set and one Agent Observability generation
- **AND** the log record and metrics SHALL carry the strongest normalized counters across both finish parts and the last finish reason
- **AND** until the Agent Observability recorder change (#582) lands, the Agent Observability generation SHALL be finalized at the first finish and carry only the first segment's usage and finish reason

### Requirement: Logical finalization precedence and strongest usage

When upstream closure and downstream context cancellation are both observable at finalization, cancellation or timeout SHALL take precedence consistently across every logical observer. Every observer SHALL pass through the original result, error, request metadata, response headers, and stream parts without mutation. Usage SHALL be recorded from the unary result or independently aggregated across every usage-bearing stream part it observes, using the established strongest-value semantics.

#### Scenario: Logical finalization precedence and strongest usage
- **WHEN** upstream closes while downstream cancellation is observable and several parts report stronger usage
- **THEN** all observers SHALL select cancellation consistently, preserve strongest counters and pass original boundary values unchanged

#### Scenario: Agent Observability stops at the first finish
- **WHEN** usage-bearing parts arrive after the first finish of a stream with several response segments
- **THEN** logs and metrics SHALL include them in their strongest-value aggregation, while the Agent Observability generation SHALL exclude them until the #582 recorder change lands
