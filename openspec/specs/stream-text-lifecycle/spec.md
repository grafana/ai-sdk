# stream-text-lifecycle Specification

## Purpose

Define `StreamText` step completion, premature provider-stream closure, partial-result retention, callback, and cancellation behavior.
## Requirements
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

### Requirement: Empty incomplete stream fails

When a provider channel closes without a terminal part and without producing any model output part, `StreamText` SHALL set a result error wrapping `ErrNoOutputGenerated`. The error SHALL state that the model stream ended without a finish chunk. Administrative stream-start, response-metadata, and raw parts SHALL NOT count as model output.

The incomplete step SHALL NOT be recorded, `OnStepFinish` SHALL NOT be called for it, `OnFinish` SHALL NOT be called for the failed invocation, and the full stream SHALL emit an error part without a synthetic successful stream-finish part.

#### Scenario: Initial step closes without output

- **WHEN** the first provider stream emits only administrative parts and closes without a terminal part
- **THEN** the result error wraps `ErrNoOutputGenerated`
- **AND** the result contains no recorded steps
- **AND** `OnStepFinish` and `OnFinish` are not called
- **AND** the full stream ends after the error part without a stream-finish part

#### Scenario: Continuation step closes without output

- **WHEN** one or more prior steps completed successfully
- **AND** a continuation provider stream emits only administrative parts and closes without a terminal part
- **THEN** the result error wraps `ErrNoOutputGenerated`
- **AND** only the prior completed steps remain recorded
- **AND** `OnStepFinish` is not called for the incomplete continuation
- **AND** `OnFinish` is not called for the failed invocation

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

### Requirement: Cancellation is not partial completion

Context cancellation while reading a provider stream SHALL use abort behavior rather than incomplete-stream finalization. Output received before cancellation SHALL NOT cause the canceled step to execute pending tools, emit `finish-step`, or be recorded unless the provider step had already completed under the existing cancellation rules.

#### Scenario: Cancellation follows a tool call

- **WHEN** a provider stream emits a tool call
- **AND** the context is canceled before a terminal part arrives
- **THEN** the tool is not executed as partial-stream completion
- **AND** no `finish-step` is emitted for the canceled step
- **AND** the canceled step is not recorded

### Requirement: Step content preserves provider order

`StreamText` SHALL construct `StepResult.Content` and response-message content from the provider's recorded content sequence. Text, reasoning, sources, regular files, provider tool calls, provider tool results, and provider approval requests SHALL retain their relative provider order and provider metadata. Tool approvals and results created locally after provider streaming SHALL be appended exactly once. Manually constructed step state without recorded provider content SHALL use deterministic grouped fallback behavior.

#### Scenario: Provider content is interleaved

- **WHEN** a provider emits files, tool calls, text, reasoning, sources, and provider tool results in an interleaved sequence
- **THEN** `StepResult.Content` and response-message content SHALL preserve that sequence
- **AND** each represented part SHALL retain its provider metadata

#### Scenario: Empty recorded text remains step content

- **WHEN** a provider emits a text start and end without a text delta
- **THEN** `StepResult.Content` SHALL retain the empty text part in its recorded position
- **AND** response-message content SHALL omit the empty text part

#### Scenario: Local tool content follows recorded content

- **WHEN** tool approval requests, tool approval responses, or client-executed tool results are created after provider streaming
- **THEN** they SHALL appear exactly once after the recorded provider content

#### Scenario: Step state has no recorded content

- **WHEN** content is built from manually constructed step fields without a recorded provider sequence
- **THEN** reasoning, text, tools, sources, and files SHALL be emitted in that grouped order

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
