# streamed-tool-results Specification

## Purpose

Define preliminary and final local tool outputs, their stream and UI delivery, and their effect on continuation.

## Requirements

### Requirement: Local tool execution can stream outputs

Tools SHALL support opting into streamed execution instead of single-result Execute. Every yield SHALL emit preliminary result; on normal completion the last yield SHALL emit again as final. No yields SHALL produce final absent output, matching upstream empty async iterable. Single-result execution SHALL emit only final result. Both forms on one tool SHALL be rejected as ambiguous.

#### Scenario: Multiple streamed values
- **WHEN** a tool streams outputs A and B and completes successfully
- **THEN** the full stream SHALL contain preliminary results A and B followed by a non-preliminary final result B for the same call

#### Scenario: Empty streamed execution
- **WHEN** a streaming tool completes without yielding an output
- **THEN** the call SHALL produce one final result with absent output and no preliminary results

#### Scenario: Single-result compatibility
- **WHEN** a tool uses only the existing `Execute` function
- **THEN** it SHALL produce one final result without a preliminary marker

### Requirement: Preliminary outputs do not finalize a call

Preliminary local results SHALL appear in full/UI streams with Preliminary true, but SHALL NOT enter final StepResult.ToolResults, ToModelOutput, continuation prompts, completion checks or terminal execution callbacks. Only final output/error SHALL be terminal for those purposes. GenerateText SHALL inherit final-result collection through StreamText.

#### Scenario: No premature continuation
- **WHEN** a streaming tool yields a preliminary result and waits before completing
- **THEN** a subsequent model step SHALL not start while the tool remains open
- **AND** the preliminary result SHALL be visible to stream consumers

#### Scenario: Final-only model output
- **WHEN** a tool yields two outputs and has a `ToModelOutput` converter
- **THEN** the converter SHALL receive only the final repeated value
- **AND** the next model prompt and final tool-results accessor SHALL contain only that final value

#### Scenario: UI preliminary marker
- **WHEN** a local tool streams preliminary results then finishes
- **THEN** UI output-available chunks SHALL carry the preliminary marker for each yielded value and omit it for the final value

### Requirement: Streaming errors and cancellation retain completed preliminary events

A streaming tool that fails after yielding preliminary values SHALL emit a terminal tool error, not a successful final value. Other concurrent tool executions SHALL continue independently. The tool SHALL receive the operation context, and a producer that honors cancellation SHALL terminate without synthesizing a successful final result. Preliminary events already delivered SHALL remain visible and SHALL not become continuation results.

#### Scenario: Error after preliminary output
- **WHEN** a tool emits one preliminary value and then returns an error
- **THEN** the stream SHALL contain that preliminary result followed by a tool error
- **AND** no successful final tool result SHALL be produced for the call

#### Scenario: Cancellation during streaming
- **WHEN** the parent context is canceled while a streaming tool is yielding
- **THEN** the tool SHALL observe the canceled context
- **AND** cancellation SHALL not turn its last preliminary result into a successful final output

### Requirement: Concurrent preliminary delivery remains serialized to consumers

Multiple local streaming tools SHALL execute concurrently. Preliminary and final/error events SHALL arrive as each tool produces them, while emission to the full stream, UI stream, and `OnChunk` callback SHALL remain serialized. Final results retained on the step SHALL preserve the existing call-order rule.

#### Scenario: Fast preliminary result from another tool
- **WHEN** tool A is still streaming and tool B produces an output
- **THEN** stream consumers SHALL be able to receive tool B's output before tool A finishes

#### Scenario: Concurrent callbacks
- **WHEN** two tool executions emit preliminary results at the same time
- **THEN** `OnChunk` SHALL not be invoked concurrently
