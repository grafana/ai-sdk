## ADDED Requirements

### Requirement: Selectable requested model identity
Agent Observability recording options SHALL provide a named identity-source setting. Its zero value SHALL preserve the current response-preferred behavior. Requested identity mode SHALL keep `GenerationStart` wrapped-model identity as the final generation model, SHALL ignore unary and streaming response identity for recording, and SHALL omit response model, provider response ID, and transport identity metadata. It SHALL observe and return all provider results and stream parts unchanged.

#### Scenario: Requested identity suppresses backend identity
- **WHEN** requested identity mode starts with provider `grafana` and model `grafana/assistant`, and a result or stream metadata names provider `anthropic`, a backend model ID, and a response ID
- **THEN** the finalized generation SHALL identify only provider `grafana` and model `grafana/assistant`
- **AND** response model, response ID, and transport identity metadata SHALL be absent
- **AND** the original result or stream part SHALL retain its response metadata

#### Scenario: Zero value preserves existing behavior
- **WHEN** identity source is not configured and complete response identity differs from wrapped model identity
- **THEN** generation identity and transport metadata SHALL retain the existing response-preferred behavior

### Requirement: Configurable bounded recording stream drain
Agent Observability recording options SHALL provide an optional positive stream-drain duration. When configured, cancellation cleanup SHALL drain the immediate upstream only until channel close or the absolute deadline, including for a continuously ready channel. Zero SHALL preserve the reconciled direct-consumer behavior of not draining after cancellation.

#### Scenario: Configured drain expires
- **WHEN** downstream cancellation occurs and the immediate upstream never closes or remains continuously ready
- **THEN** the Agent Observability-owned drain goroutine SHALL exit no later than the configured absolute deadline
- **AND** generation finalization and downstream channel closure SHALL each occur exactly once without waiting for the drain

### Requirement: Consumer-owned final generation policy
Agent Observability recording options SHALL provide an optional generation filter invoked once for every mapped unary or streaming generation, including a successful nil stream. The input SHALL include the mapped generation and the observed provider finish reason. A nil filter SHALL preserve existing mapping behavior. The filter SHALL NOT mutate provider call options, results, or stream parts. A filter panic SHALL be recovered, SHALL produce only a minimal generation seeded with requested model identity, and SHALL NOT change the model call result.

#### Scenario: Gateway filter drops mapper-only private metadata
- **WHEN** a consumer filter receives a generation containing provider-option metadata, raw usage metadata, response identity, tags, or a provider-native finish reason
- **THEN** the filter MAY retain only its approved normalized observation fields and the unified finish value
- **AND** the original model result or stream parts SHALL remain unchanged

#### Scenario: Filter panic remains fail-open
- **WHEN** a configured generation filter panics
- **THEN** recording SHALL finalize a minimal requested-identity generation or report a local recording error
- **AND** the model call result, error, or stream SHALL remain unchanged

### Requirement: Provided-only recording context
Agent Observability recording options SHALL provide a context-source mode whose zero value preserves existing ambient fallbacks. Provided-only mode SHALL retain cancellation, deadlines, and the active OpenTelemetry parent while preventing the Agent Observability client and mapper from reading arbitrary request values or Agent Observability conversation, user, agent, tag, experiment, generation, and parent-generation context helpers. The original context SHALL still reach the wrapped model.

#### Scenario: Ambient observation values are isolated
- **WHEN** provided-only mode receives a context containing Agent Observability values and an unrelated provider value
- **THEN** the exported generation and span SHALL omit the ambient Agent Observability values
- **AND** the provider SHALL still receive its unrelated value, cancellation, deadline, and the generated recording span

### Requirement: Fail-open local recording diagnostics
Agent Observability recording options SHALL provide an optional record-error handler invoked after finalization when local validation or enqueueing fails and an optional completion handler invoked once after every started recorder ends. Handler panics SHALL be recovered, and neither recorder failure nor handler failure SHALL alter the model call result or stream. Streaming recorders SHALL end before downstream EOF while completion/error callbacks SHALL NOT delay that EOF.

#### Scenario: Queue failure is reported without failing traffic
- **WHEN** a recorder cannot enqueue a finalized generation
- **THEN** the handler SHALL receive the local error once
- **AND** the original model response SHALL remain unchanged even if the handler panics

#### Scenario: Process owner tracks an abandoned stream
- **WHEN** a stream consumer stops reading and request cancellation ends its recorder asynchronously
- **THEN** the completion handler SHALL run exactly once after the generation is finalized
- **AND** a blocking completion or error handler SHALL NOT delay downstream EOF
