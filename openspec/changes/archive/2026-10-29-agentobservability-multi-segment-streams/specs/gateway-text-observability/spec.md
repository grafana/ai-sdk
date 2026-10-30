## ADDED Requirements

### Requirement: Agent Observability generation covers every response segment
A streaming text call SHALL produce one Agent Observability generation per stream. Its recorder SHALL observe every part of every response segment and finalize once, at stream close or when an idle timeout, the request deadline or cancellation ends it, not at a finish. The generation SHALL carry the last finish's segment usage under shared streaming usage aggregation, the last finish reason, first-output timing from the first segment and, on normal close, completion at stream end.

#### Scenario: Two segments close normally
- **WHEN** a stream emits two response segments, each ending with a finish that reports its own usage and finish reason, and then closes
- **THEN** the request SHALL produce one logical log record, one metric observation set and one Agent Observability generation
- **AND** the generation, log record and metrics SHALL carry the same usage, the second segment's, and the second finish reason
- **AND** the generation's completion time SHALL be the stream end, and its first-output timing SHALL come from the first segment

### Requirement: Agent Observability generation after a finish
When an idle timeout, the request deadline or cancellation ends a stream after at least one finish, cancellation or timeout SHALL NOT be the Agent Observability generation's call error; the latest provider stream error observed before or after the last finish SHALL be, if any. With no provider stream error after the last finish, the generation SHALL record the state at that finish, completed when that finish was observed; otherwise it SHALL record the state observed so far.

#### Scenario: Stall after a finished segment
- **WHEN** a stream emits one finished segment and then stalls until the idle timeout, the request deadline or cancellation ends it
- **THEN** logs and metrics SHALL keep the timeout or cancellation classification
- **AND** the Agent Observability generation SHALL record the finished segment's usage and finish reason without a cancellation or timeout call error, completed when that finish was observed
- **AND** its usage SHALL equal the usage that logs and metrics report, because parts after the last finish do not change shared streaming usage

#### Scenario: Provider error after a finish, then a stall
- **WHEN** a stream emits a finished segment, then a provider error part, and then stalls until the idle timeout, the request deadline or cancellation ends it
- **THEN** the Agent Observability generation SHALL carry that provider-error classification as its call error, not the timeout or cancellation

#### Scenario: Stream error precedes a later finish, then a stall
- **WHEN** a stream emits a provider error part, later a finish, and then stalls until the idle timeout, the request deadline or cancellation
- **THEN** the Agent Observability generation SHALL keep the provider-error classification as its call error
- **AND** it SHALL record the usage and finish reason observed through that finish

### Requirement: Logical finalization precedence across response segments
When upstream closure and downstream cancellation are both observable at finalization, cancellation or timeout SHALL take precedence for logs, metrics and a generation that observed no finish; a generation that observed a finish SHALL follow "Agent Observability generation after a finish". Observers SHALL pass results, errors, request metadata, response headers and stream parts through unchanged, with unary usage or shared streaming usage.

#### Scenario: Logical finalization precedence and strongest usage
- **WHEN** upstream closes while downstream cancellation is observable, no finish was observed, and several parts report stronger usage
- **THEN** all observers SHALL select cancellation consistently, preserve the strongest counters and pass original boundary values unchanged

#### Scenario: Usage after a finish reaches every observer
- **WHEN** usage-bearing parts arrive after a finish of a stream with several response segments, a later finish closes their segment, and the stream then closes normally
- **THEN** logs, metrics and the Agent Observability generation SHALL all report that later segment's usage

#### Scenario: Cancellation after a finish
- **WHEN** downstream cancellation is observable at finalization after a finish was observed
- **THEN** logs and metrics SHALL select cancellation
- **AND** the Agent Observability generation SHALL record the finished segments with the last finish's segment usage and no cancellation or timeout call error

## MODIFIED Requirements

### Requirement: Logical text lifecycle semantics

Unary observation SHALL start immediately before the shared model invocation and finalize once after its result or pre-result error. Streaming observation SHALL start immediately before stream setup. Logs and metrics SHALL finalize once after normal channel close, provider error observation, premature close, downstream cancellation, or timeout as represented at the model boundary. A stream with several response segments SHALL remain one logical call.

#### Scenario: Stream error precedes later finish
- **WHEN** a stream emits usage, a provider error part, later text, and finish before closing
- **THEN** every part SHALL pass through unchanged and in order
- **AND** logical observers SHALL finalize once with the established error classification and the same usage under shared streaming usage aggregation
- **AND** middleware SHALL not terminate the stream or trigger fallback

#### Scenario: Usage is split across stream parts
- **WHEN** different stream parts of one response segment report different usage counters
- **THEN** the terminal logical log, metrics and Agent Observability generation SHALL carry the same usage: the counters the segment's finish reports and, for the counters it omits, the strongest values observed in that segment

#### Scenario: Stream with several response segments
- **WHEN** a stream emits two response segments, each ending with a finish that reports its own usage and finish reason, and then closes
- **THEN** every part SHALL pass through unchanged and in order, and the request SHALL produce one logical log record, one metric observation set and one Agent Observability generation
- **AND** the log record, metrics and Agent Observability generation SHALL carry the last finish's segment usage and the last finish reason

## REMOVED Requirements

### Requirement: Agent Observability first-segment limitation
**Reason**: The Agent Observability recorder no longer finalizes at a stream's first finish. It observes every response segment and finalizes once, at stream close or when an idle timeout, the request deadline or cancellation ends the stream, so parts after the first finish are no longer excluded from the generation.
**Migration**: Use "Agent Observability generation covers every response segment" for when the generation finalizes and what it carries, and "Agent Observability generation after a finish" for its call error and recorded state when a stall or cancellation follows a finish. Consumers that read the exported generation see the last finish reason and the last finish's segment usage instead of the first segment's values.

### Requirement: Logical finalization precedence and strongest usage
**Reason**: Its scenario "Agent Observability stops at the first finish" required the generation to exclude parts after the first finish, and its precedence rule made cancellation or timeout the generation's call error even after a finished segment. Neither holds once the recorder observes every segment and keeps a finished segment's state on a later stall or cancellation.
**Migration**: Use "Logical finalization precedence across response segments". It keeps the precedence, pass-through and strongest-usage rules and the "Logical finalization precedence and strongest usage" scenario, defers a generation that observed a finish to "Agent Observability generation after a finish", and replaces the first-finish scenario with "Usage after a finish reaches every observer".
