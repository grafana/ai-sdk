## MODIFIED Requirements

### Requirement: Frontend hook interop coverage
The repository SHALL include hook-level frontend interop tests for upstream AI SDK UI consumers at the package versions registered in `test/conformance/upstream.yaml`. A behavior SHALL count as hook-level evidence only when an assertion executes through the corresponding public upstream hook surface; lower-level chunk snapshots, SSE parsing, or UI-message reader tests SHALL NOT substitute for hook state-machine evidence.

#### Scenario: React chat hook consumes Go UI stream
- **WHEN** integration tests run
- **THEN** `useChat`-level tests verify Go SSE consumption through the upstream React hook surface
- **AND** covered tests include successful status ordering, HTTP and stream errors, stop with retained partial output, a multi-step boundary, and approved and denied tool approval responses

#### Scenario: React completion hook consumes Go streams
- **WHEN** integration tests run
- **THEN** `useCompletion`-level tests cover successful consumption, error callback and loading reset, and stop with retained partial completion

#### Scenario: React object hook consumes and validates Go streams
- **WHEN** integration tests run
- **THEN** `useObject`-level tests cover a successful object and a completed schema-invalid object
- **AND** the schema-invalid test asserts the final `onFinish` result

#### Scenario: Lower-level evidence remains distinct
- **WHEN** a parity claim is supported only by chunk snapshots, SSE parsing, or UI-message reader tests
- **THEN** the coverage map identifies that proof as lower-level evidence rather than hook-level state-machine coverage

## ADDED Requirements

### Requirement: Frontend parity status reflects covered breadth
Frontend capability statuses in `test/conformance/PARITY.md` SHALL reflect the breadth of behavior directly exercised at the stated layer. A broad capability with automated coverage for only a subset of its state transitions or failures MUST be classified as `mixed`, with the automated subset and remaining gap described in its confidence source or notes.

#### Scenario: Hook surfaces have partial state-machine coverage
- **WHEN** the suite automates the hook scenarios required by this change but does not exhaustively cover each public hook lifecycle and failure path
- **THEN** the broad `useChat`, `useCompletion`, and `useObject` compatibility rows are classified as `mixed`
- **AND** each row describes the behavior that is automated through the hook

#### Scenario: Chunk ordering evidence is broader than hook evidence
- **WHEN** conformance snapshots automate chunk ordering for fixtures but React assertions cover only selected state transitions
- **THEN** the broad chunk ordering and state transitions row is classified as `mixed`
- **AND** its notes distinguish conformance stream ordering from React hook state transitions
