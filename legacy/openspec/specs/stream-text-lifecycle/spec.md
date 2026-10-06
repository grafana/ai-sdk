# stream-text-lifecycle Specification

## Purpose

Define `StreamText` step completion, premature provider-stream closure, partial-result retention, callback, and cancellation behavior.

## Requirements

### Requirement: Provider stream terminal recognition

`StreamText` SHALL distinguish provider terminal parts from model output and administrative parts. A provider finish part SHALL complete the step normally. A provider error part SHALL be terminal for premature-closure detection and SHALL retain its existing error handling. Closing the provider channel without either terminal part SHALL be treated as an incomplete stream.

#### Scenario: Provider finish completes a step

- **WHEN** a provider stream emits a finish part and closes
- **THEN** `StreamText` records the step and emits the normal step and stream completion parts

#### Scenario: Provider error is terminal

- **WHEN** a provider stream emits an error part and then closes without a finish part
- **THEN** `StreamText` surfaces the provider error
- **AND** it does not replace that error with a premature-closure error

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

When a provider channel closes without a terminal part after producing a model output part, `StreamText` SHALL retain the partial step instead of reporting `ErrNoOutputGenerated`. The step finish reason SHALL be `other`, normal step finalization and `OnStepFinish` SHALL run, and the stream SHALL emit `finish-step`. Existing tool-loop continuation rules MAY start another provider step when the partial output contains executable client tool calls. If the invocation does not continue, it SHALL emit a stream finish whose reason is `other`.

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

#### Scenario: Partial tool-call step continues

- **WHEN** a non-empty incomplete provider stream contains executable client tool calls
- **AND** existing stop and continuation conditions allow another step
- **THEN** the partial step is recorded with finish reason `other`
- **AND** `StreamText` MAY execute the tools and start the continuation step

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
