# Spec Delta

## ADDED Requirements

### Requirement: Unsupported functionality sentinel
The provider package SHALL export a sentinel error for deterministic adapter rejection of a registered feature before any provider I/O. Adapters SHALL wrap it with context. It SHALL be distinct from APICallError and SHALL NOT carry an HTTP status or retryability.

#### Scenario: Core retry does not repeat a rejection
- **WHEN** StreamText or GenerateText receives a wrapped unsupported-functionality error from a model call
- **THEN** the call SHALL fail after one attempt without retry delay

#### Scenario: Wrapped identity survives
- **WHEN** an adapter returns the sentinel wrapped with contextual messages
- **THEN** errors.Is SHALL identify the sentinel and errors.As SHALL NOT find an APICallError

### Requirement: Default fallback advances past unsupported functionality
The default fallback decider SHALL treat a wrapped unsupported-functionality error as an eligible pre-commit failure and try the next candidate. A configured decider SHALL remain authoritative. Exhausted attempts SHALL keep every sentinel identity in the returned error.

#### Scenario: Next candidate serves the request
- **WHEN** the first candidate rejects with the sentinel and the second succeeds, in unary or streaming setup
- **THEN** the default decider SHALL advance once and return the second candidate's result

#### Scenario: All candidates reject
- **WHEN** every candidate rejects with the sentinel
- **THEN** the returned error SHALL satisfy errors.Is for the sentinel and each candidate SHALL have been invoked once

#### Scenario: Custom decider stops
- **WHEN** a configured decider returns false for the sentinel
- **THEN** no later candidate SHALL be invoked
