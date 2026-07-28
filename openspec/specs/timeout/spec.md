## Purpose

Define total, per-step, and per-chunk timeout behavior across language-model operations, including retry cancellation and context propagation.

## Requirements

### Requirement: Timeout configuration option
The system SHALL accept a `Timeout` option on both `StreamText` and `GenerateText` with three optional duration fields: `Total`, `Step`, and `Chunk`. All fields SHALL default to zero (disabled).

#### Scenario: No timeout configured
- **WHEN** `StreamText` is called without a `Timeout` option
- **THEN** the system SHALL NOT impose any additional timeouts beyond the caller's context

#### Scenario: Partial timeout configured
- **WHEN** only `Step` timeout is set
- **THEN** the system SHALL apply per-step timeout without a total or chunk timeout

### Requirement: Total timeout
When `Total` is set, the system SHALL abort the entire operation (all steps) if the total elapsed time exceeds this duration. Abort SHALL propagate via context cancellation.

#### Scenario: Total timeout exceeded
- **WHEN** `Total` is set to 30s and the operation takes longer than 30s across multiple steps
- **THEN** the system SHALL cancel the context and abort the operation

#### Scenario: Total timeout covers all steps
- **WHEN** `Total` is set to 10s, step 1 takes 6s, and step 2 takes 6s
- **THEN** the system SHALL abort during step 2 because total time exceeds 10s

### Requirement: Step timeout
When `Step` is set, the system SHALL abort the current model invocation (`DoStream`/`DoGenerate`) if it exceeds this duration. The step timeout SHALL reset for each step in a multi-step loop. Once a step timeout fires, the entire operation SHALL abort.

#### Scenario: Step timeout exceeded
- **WHEN** `Step` is set to 5s and a single `DoStream` call takes longer than 5s
- **THEN** the system SHALL cancel the model invocation via context cancellation

#### Scenario: Step timeout resets between steps
- **WHEN** `Step` is set to 5s, step 1 completes in 4s, and step 2 starts
- **THEN** step 2 SHALL have a fresh 5s timeout starting from when it begins

#### Scenario: Step timeout aborts entire operation
- **WHEN** `Step` timeout fires during step 2 of a multi-step tool-use loop
- **THEN** the entire operation SHALL abort, not just the current step

### Requirement: Chunk timeout (stall detection)
When `Chunk` is set, the system SHALL abort the operation if no new stream part is received within this duration during streaming. The timer SHALL reset on each received stream part. This timeout SHALL only apply to `StreamText`, not `GenerateText`.

#### Scenario: Chunk timeout triggers on stall
- **WHEN** `Chunk` is set to 10s and no stream part arrives for 10s during active streaming
- **THEN** the system SHALL cancel the operation via context cancellation

#### Scenario: Chunk timeout resets on each part
- **WHEN** `Chunk` is set to 10s and stream parts arrive every 8s
- **THEN** the system SHALL NOT trigger the chunk timeout because the timer resets on each part

#### Scenario: Chunk timeout not applied to GenerateText
- **WHEN** `Chunk` is set and `GenerateText` is called
- **THEN** the system SHALL ignore the `Chunk` timeout since `GenerateText` does not stream

### Requirement: Timeout cancels retry
When any timeout fires, it SHALL cancel the current context, which SHALL prevent any further retry attempts. Retry delays that are in progress SHALL be interrupted immediately by context cancellation.

#### Scenario: Step timeout during retry delay
- **WHEN** `Step` is set to 5s, the first `DoStream` call fails after 3s with a retryable error, and the 2s backoff delay is in progress
- **THEN** the step timeout SHALL fire at 5s, cancelling the backoff delay and aborting the operation

#### Scenario: Total timeout prevents retries
- **WHEN** `Total` is set to 10s, the first attempt fails at 8s with a retryable error
- **THEN** the retry delay and second attempt SHALL be aborted when total timeout fires at 10s

### Requirement: Timeout uses context cancellation
All timeout levels SHALL be implemented using Go `context.WithTimeout` or `context.WithCancel` with timers. The cancelled context SHALL propagate to `DoStream`/`DoGenerate` calls and through any in-progress retry delays.

#### Scenario: Context hierarchy
- **WHEN** both `Total` and `Step` timeouts are set
- **THEN** the step context SHALL be a child of the total context, so total timeout cancellation automatically cancels the step
