## ADDED Requirements

### Requirement: Stream-wide text and reasoning identifiers
StreamText SHALL preserve the first occurrence of each provider text/reasoning
identifier and reserve a unique replacement for later collisions across steps.
Text and reasoning SHALL use independent identifier namespaces. Matching deltas
and end parts SHALL use the reserved start identifier; generated collisions SHALL
receive deterministic numeric suffixes without repeatedly invoking the generator.

#### Scenario: Provider reuses IDs across steps
- **WHEN** multiple steps each emit text or reasoning with the same provider ID
- **THEN** their emitted start identifiers are unique within that content family
- **AND** deltas and ends identify their corresponding starts
- **AND** the frontend assembles separate text/reasoning parts without overwriting prior steps

#### Scenario: Custom generator also collides
- **WHEN** the replacement generator returns an identifier that is already reserved
- **THEN** a numeric suffix is advanced until the identifier is unique

### Requirement: Tool execution requires an allowed finish

Local executable tools SHALL be dispatched only after a step finishes with `stop`
or `tool-calls`. Tool-call visibility and approval processing SHALL be retained
for other finish reasons. Automatic continuation SHALL require each client tool
call to have a matching result or denial, not merely an executable callback.

#### Scenario: Recovered calls follow a malformed provider event
- **WHEN** malformed provider data and otherwise valid tool calls result in an error finish
- **THEN** the calls remain visible without invoking executable callbacks
- **AND** unresolved calls prevent a subsequent model request

#### Scenario: Approval observations precede dispatch eligibility
- **WHEN** a disallowed finish contains calls requiring approval or automatic decisions
- **THEN** approval requests and decisions remain observable
- **AND** approved calls are not executed under that finish
- **AND** matching denials may satisfy continuation without executing a tool

#### Scenario: Allowed finishes execute normally
- **WHEN** a step finishes with stop or tool-calls and contains eligible local calls
- **THEN** callbacks execute and matching results may permit the next step

## MODIFIED Requirements

### Requirement: Partial incomplete stream is retained

When a provider channel closes without a terminal part after producing a model output part, `StreamText` SHALL retain the partial step instead of reporting `ErrNoOutputGenerated`. The step finish reason SHALL be `other`, normal step finalization and `OnStepFinish` SHALL run, and the stream SHALL emit `finish-step`. Continuation SHALL require every client tool call to have a result or denial. Incomplete steps SHALL NOT dispatch executable tools. If the invocation does not continue, it SHALL emit a stream finish whose reason is `other`.

Model output classification SHALL follow the registered upstream baseline so output-bearing parts are distinguished from administrative, raw, finish, and error parts.

#### Scenario: Text output precedes premature closure

- **WHEN** a provider stream emits text output and closes without a terminal part
- **THEN** the partial text is available from the result
- **AND** the result has no premature-closure error
- **AND** the partial step is recorded with finish reason `other`
- **AND** `OnStepFinish` is called for that step

#### Scenario: Partial step stream lifecycle without continuation

- **WHEN** a non-empty incomplete provider stream is finalized
- **AND** existing tool-loop rules do not start another provider step
- **THEN** the full stream emits `finish-step`
- **AND** it emits a stream finish with finish reason `other`

#### Scenario: Partial tool-call step retains unresolved calls

- **WHEN** a non-empty incomplete provider stream contains executable client tool calls without results
- **THEN** the partial step is recorded with finish reason `other`
- **AND** `StreamText` does not execute the tools or start a continuation step
