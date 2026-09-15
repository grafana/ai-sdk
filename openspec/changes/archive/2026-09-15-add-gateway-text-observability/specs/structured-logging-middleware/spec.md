## ADDED Requirements

### Requirement: Selectable requested model identity
Structured logging options SHALL provide a named identity-source setting. Its zero value SHALL preserve the current response-preferred behavior. Requested identity mode SHALL use the wrapped model provider and model ID for start and terminal records, SHALL NOT replace them from unary or streaming response metadata, and SHALL omit response/transport identity fields including provider response IDs. It SHALL observe and return all response metadata unchanged.

#### Scenario: Requested identity suppresses backend identity
- **WHEN** requested identity mode wraps provider `grafana` and model `grafana/assistant`, and a response names provider `anthropic`, a backend model ID, and a response ID
- **THEN** start and terminal records SHALL identify only provider `grafana` and model `grafana/assistant`
- **AND** response and transport identity attributes SHALL be absent
- **AND** the original result or stream part SHALL retain its response metadata

#### Scenario: Zero value preserves existing behavior
- **WHEN** identity source is not configured and complete response identity differs from wrapped model identity
- **THEN** terminal identity and transport metadata SHALL retain the existing response-preferred behavior

### Requirement: Configurable bounded stream drain
Structured logging options SHALL provide an optional positive stream-drain duration. When configured, cancellation cleanup SHALL drain the immediate upstream only until channel close or the absolute deadline, including for a continuously ready channel. Zero SHALL preserve the existing direct-consumer drain behavior.

#### Scenario: Configured drain expires
- **WHEN** downstream cancellation occurs and the immediate upstream never closes or remains continuously ready
- **THEN** the logger-owned drain goroutine SHALL exit no later than the configured absolute deadline
- **AND** terminal logging and downstream channel closure SHALL occur exactly once without waiting for the drain
