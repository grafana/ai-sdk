## MODIFIED Requirements

### Requirement: Provider stream terminal recognition

`StreamText` SHALL distinguish provider terminal parts from model output and administrative parts. A provider finish part SHALL complete the model call; a response satisfying applicable semantic validation SHALL complete the step normally, while an effective required/named tool-choice violation SHALL use completed semantic-failure finalization. A provider error part SHALL be terminal for premature-closure detection and SHALL retain its existing error handling. Closing the provider channel without either terminal part SHALL be treated as an incomplete stream.

#### Scenario: Provider finish completes a step

- **WHEN** a provider stream emits a finish part and closes with no tool-choice violation
- **THEN** `StreamText` records the step and emits the normal step and stream completion parts

#### Scenario: Provider error is terminal

- **WHEN** a provider stream emits an error part and then closes without a finish part
- **THEN** `StreamText` surfaces the provider error
- **AND** it does not replace that error with a premature-closure or missing-required-call error

#### Scenario: Provider finish permits semantic failure with retained completion

- **WHEN** a provider finish completes a response violating effective required or named choice
- **THEN** the completed model call SHALL be retained and finalized as a semantic failure, not discarded as an unfinished or ordinary processing-error step

### Requirement: Tool execution requires an allowed finish

Local executable tools SHALL be dispatched only after a step finishes with `stop`
or `tool-calls` and does not violate its effective required/named tool choice. Tool-call visibility SHALL be retained for other finish reasons. Approval processing SHALL be retained for other finish reasons except completed tool-choice violations, which SHALL bypass all new local approval and execution processing. Automatic continuation SHALL require each client tool call to have a matching result or denial, not merely an executable callback, and SHALL never occur after a tool-choice violation.

#### Scenario: Recovered calls follow a malformed provider event
- **WHEN** malformed provider data and otherwise valid tool calls result in an error finish without a completed tool-choice violation
- **THEN** the calls remain visible without invoking executable callbacks
- **AND** unresolved calls prevent a subsequent model request

#### Scenario: Approval observations precede dispatch eligibility
- **WHEN** a disallowed finish contains calls requiring approval or automatic decisions without a completed tool-choice violation
- **THEN** approval requests and decisions remain observable
- **AND** approved calls are not executed under that finish
- **AND** matching denials may satisfy continuation without executing a tool

#### Scenario: Allowed finishes execute normally
- **WHEN** a step finishes with stop or tool-calls, satisfies its applicable tool choice, and contains eligible local calls
- **THEN** callbacks execute and matching results may permit the next step

#### Scenario: Choice violation bypasses local processing
- **WHEN** a completed response violates effective required or named choice
- **THEN** no new local approval processing or executable callback SHALL run for that step
- **AND** the received tool calls SHALL remain visible
- **AND** no subsequent model request SHALL start

## ADDED Requirements

### Requirement: Completed tool-choice failures retain model-call data

On a completed tool-choice violation, shared orchestration SHALL preserve the received usage, raw finish reason, response metadata, provider metadata, warnings and original content. It SHALL replace the step's unified finish reason with `error`, build the normal completed content/response-message projection, emit `finish-step` and record the failed completed step before terminal invocation finalization. Last-step/response/provider-metadata accessors SHALL reflect that failed completed call. Prior completed steps SHALL remain recorded, and total usage SHALL include them and the failed completed call.

The failure SHALL be surfaced exactly once as a tool-choice `StreamError`, stored result error and `OnError` notification. Existing `OnStepFinish` and `OnFinish` callbacks SHALL run once for the completed failed step and invocation respectively with retained data. The final stream finish SHALL have unified reason `error` and retained aggregate usage, not an empty successful finish. The invocation SHALL not retry or continue after this semantic failure, regardless of stop conditions or client/provider call resolution.

`GenerateText` and `ToolLoopAgent.Generate` SHALL retain their existing `nil, error` return contract. Their existing completion callbacks SHALL expose the retained completed-call data; no partial-result or new error API SHALL be required. `ToolLoopAgent.Stream` SHALL retain the same streaming result behavior as `StreamText`.

#### Scenario: Required no-call failure retains completed data
- **WHEN** a required-choice call finishes with text/reasoning, usage, raw finish and response/provider metadata but no parsed tool call
- **THEN** the failed completed step and its original content SHALL be recorded with unified finish error and unchanged raw finish
- **AND** accessors, finish-step and completion callbacks SHALL retain usage and metadata
- **AND** one semantic error and a final finish with reason error and total usage SHALL be observable

#### Scenario: Violation after prior successful steps
- **WHEN** one or more steps succeeded before a completed choice violation
- **THEN** all prior steps and the failed completed step SHALL remain recorded in order
- **AND** final total usage SHALL include every completed call
- **AND** no retry or continuation SHALL follow the failed step

#### Scenario: Resolved or provider-only calls do not permit continuation
- **WHEN** a named-choice violation contains unrelated provider-executed calls or client calls already resolved by provider results
- **THEN** the semantic failure SHALL still terminate the invocation
- **AND** no additional model call SHALL occur even when stop conditions permit more steps

#### Scenario: Generate and Agent preserve existing return contracts
- **WHEN** GenerateText or ToolLoopAgent.Generate encounters a completed tool-choice violation
- **THEN** it SHALL return nil result and the semantic error
- **AND** existing completion callbacks SHALL receive the failed completed step, retained usage/metadata and aggregate totals
- **AND** direct calls to both streaming entry points SHALL expose equivalent failure and retained data through existing results

#### Scenario: Incomplete and canceled calls are not choice violations
- **WHEN** required or named choice is active but the stream ends without a provider finish, reports a provider error, or is canceled before completing
- **THEN** existing empty/partial/error/abort behavior SHALL remain unchanged
- **AND** no missing-required-call semantic error SHALL replace that outcome
