## ADDED Requirements

### Requirement: Agent Observability generation covers every response segment
A streaming text call whose provider stream carries several response segments SHALL produce one Agent Observability generation covering every segment. It SHALL carry the strongest normalized usage counters across all segments, the last finish reason, first-output timing from the first segment, and, on normal close, a completion time at stream end. On normal close its usage SHALL equal the totals that logs and metrics report.

#### Scenario: Two segments close normally
- **WHEN** a stream emits two response segments, each ending with a finish that reports its own usage and finish reason, and then closes
- **THEN** the request SHALL produce one logical log record, one metric observation set and one Agent Observability generation
- **AND** the generation, log record and metrics SHALL carry the same strongest normalized counters across both finishes and the second finish reason

#### Scenario: Stall after a finished segment
- **WHEN** a stream emits one finished segment and then stalls until the idle timeout, the request deadline or cancellation ends it
- **THEN** logs and metrics SHALL keep the timeout or cancellation classification
- **AND** the Agent Observability generation SHALL record the finished segment's usage and finish reason without a cancellation or timeout call error
- **AND** its usage MAY differ from logs and metrics, which keep aggregating parts after that finish

## MODIFIED Requirements

### Requirement: Logical finalization precedence and strongest usage

When upstream closure and downstream context cancellation are both observable at finalization, cancellation or timeout SHALL take precedence consistently across every logical observer, except for an Agent Observability generation that already observed a finish. Its call error SHALL NOT be cancellation or timeout; it SHALL be the latest provider stream error observed before or after the last finish, if any. It SHALL record the state at the last finish when no stream error followed that finish, and the state observed so far otherwise. Every observer SHALL pass through the original result, error, request metadata, response headers, and stream parts without mutation. Usage SHALL be recorded from the unary result or independently aggregated across every usage-bearing stream part using the established strongest-value semantics, except that an Agent Observability generation recorded from the last-finish state carries usage aggregated through that finish.

#### Scenario: Logical finalization precedence and strongest usage
- **WHEN** upstream closes while downstream cancellation is observable, no finish was observed, and several parts report stronger usage
- **THEN** all observers SHALL select cancellation consistently, preserve strongest counters and pass original boundary values unchanged

#### Scenario: Cancellation after a finish
- **WHEN** downstream cancellation is observable at finalization after a finish was observed
- **THEN** logs and metrics SHALL select cancellation
- **AND** the Agent Observability generation SHALL record the finished segments with strongest counters and no cancellation or timeout call error

#### Scenario: Stream error precedes a later finish, then a stall
- **WHEN** a stream emits a provider error part, later a finish, and then stalls until the idle timeout, the request deadline or cancellation
- **THEN** the Agent Observability generation SHALL keep the provider-error classification as its call error
