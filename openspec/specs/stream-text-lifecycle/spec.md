# stream-text-lifecycle Specification

## Purpose

Define `StreamText` step completion, premature provider-stream closure, partial-result retention, callback, and cancellation behavior.
## Requirements
### Requirement: Provider stream terminal recognition

StreamText SHALL distinguish terminal parts from model/admin output. Finish SHALL complete the model call: semantically valid responses SHALL complete normally; effective required/named tool-choice violations SHALL use completed semantic-failure finalization. Provider error SHALL be terminal for premature-closure detection with existing handling. Closure without finish/error SHALL be incomplete.

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

Closure without terminal or model output SHALL set a result error wrapping ErrNoOutputGenerated, stating the model stream ended without a finish chunk. Stream-start/response-metadata/raw parts SHALL NOT count as model output. The step SHALL NOT be recorded; OnStepFinish/OnFinish SHALL NOT run for the incomplete step/failed invocation. Full stream SHALL emit error without synthetic successful finish.

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

Closure without terminal after model output SHALL retain a partial step, not ErrNoOutputGenerated. Finish reason SHALL be other; normal finalization/OnStepFinish SHALL run and emit finish-step. Continuation SHALL require results/denials for every client call. Incomplete steps SHALL NOT dispatch tools; if not continued, stream finish SHALL use other.

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

### Requirement: Incomplete output classification follows the baseline

Model output classification SHALL follow the registered upstream baseline, distinguishing output-bearing parts from administrative, raw, finish and error parts.

#### Scenario: Administrative-only incomplete stream has no model output
- **WHEN** a stream contains only stream-start, response-metadata or raw parts before premature closure
- **THEN** those parts SHALL NOT turn the empty incomplete stream into a retained partial-output step.

### Requirement: Cancellation is not partial completion

Context cancellation while reading a provider stream SHALL use abort behavior rather than incomplete-stream finalization. Output received before cancellation SHALL NOT cause the canceled step to execute pending tools, emit `finish-step`, or be recorded unless the provider step had already completed under the existing cancellation rules.

#### Scenario: Cancellation follows a tool call

- **WHEN** a provider stream emits a tool call
- **AND** the context is canceled before a terminal part arrives
- **THEN** the tool is not executed as partial-stream completion
- **AND** no `finish-step` is emitted for the canceled step
- **AND** the canceled step is not recorded

### Requirement: Step content preserves provider order

StreamText SHALL build StepResult.Content and response content from recorded provider sequence. Text/reasoning/sources/regular files/tool calls/tool results/approval requests SHALL retain relative provider order and metadata. Locally created post-stream approvals/results SHALL append exactly once. Manually constructed state without recorded content SHALL use deterministic grouped fallback.

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

Local executable tools SHALL dispatch only after stop/tool-calls without effective required/named choice violation. Other finish reasons SHALL retain call visibility and approval processing, except completed choice violations SHALL bypass all new local approval/execution. Continuation SHALL require matching result/denial for every client call, not merely a callback, and SHALL never follow a choice violation.

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

Completed choice violations SHALL retain usage, raw finish, response/provider metadata, warnings and original content, set unified step finish to error, build normal content/response projections, emit finish-step and record the failed completed step before terminal finalization. Last-step/response/provider-metadata accessors SHALL reflect it. Prior steps SHALL remain recorded; total usage SHALL include all completed calls.

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

### Requirement: Completed choice failure notifications and termination

Choice failure SHALL surface exactly once as tool-choice StreamError, stored result error and OnError. OnStepFinish/OnFinish SHALL run once for the failed completed step/invocation with retained data. Final finish SHALL use unified error and retained aggregate usage, not empty success. No retry/continuation SHALL follow regardless of stop conditions or client/provider call resolution.

#### Scenario: Semantic failure has one notification per surface
- **WHEN** a completed choice failure reaches invocation finalization
- **THEN** StreamError, result error and OnError SHALL each expose it once, with one completion callback per step/invocation and error finish with total usage.

### Requirement: Generated choice failures retain existing error APIs

GenerateText and ToolLoopAgent.Generate SHALL retain nil, error returns; existing completion callbacks SHALL expose retained completed-call data. No partial-result or new error API SHALL be required. ToolLoopAgent.Stream SHALL retain StreamText streaming-result behavior.

#### Scenario: Generate choice failure remains nil result and error
- **WHEN** a generated Agent call completes with a choice violation
- **THEN** it SHALL return nil, error while existing completion callbacks retain completed-call data without a new partial-result API.

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

### Requirement: Cancellation releases an unread stream

Stopping FullStream reads SHALL NOT keep a canceled run alive. Parts fitting the buffer SHALL be delivered even after cancellation, including abort for reading consumers. When full and context canceled by caller or total/step/first-chunk/chunk timeout, a part SHALL be dropped so the run goroutine returns, FullStream closes, and Wait/blocking accessors return.

#### Scenario: Abandoned stream finishes after cancellation

- **WHEN** a consumer reads one part from `FullStream`, stops reading until the buffer is full, and cancels the context
- **THEN** the run goroutine returns, `FullStream` is closed, and `Wait()` returns

#### Scenario: Abandoned stream finishes after a timeout

- **WHEN** a consumer reads one part from `FullStream`, stops reading until the buffer is full, and a configured total timeout then expires without the caller canceling
- **THEN** the run goroutine returns, `FullStream` is closed, and `Wait()` returns

#### Scenario: A reading consumer still receives post-cancellation parts

- **WHEN** the context is canceled while a consumer is still reading `FullStream` and the buffer has room
- **THEN** the `abort` part is delivered to that consumer and `FullStream` then closes, with no `finish` part, as for any canceled stream
