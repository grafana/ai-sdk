## Purpose

Concurrent execution of multiple tool calls within a single step, matching upstream Vercel AI SDK `Promise.all` semantics. Tools execute in parallel via goroutines, completing in the time of the slowest tool rather than the sum of all tool execution times.

## Requirements

### Requirement: Concurrent tool execution within a step

For multiple calls in one step, `executeTools()` SHALL run each eligible call in its own goroutine and wait for all to complete. Eligibility SHALL require ProviderExecuted false and a tool in the effective execution set with non-nil Execute or configured streaming execution. Existing allowed-finish and approval gates SHALL continue to apply.

#### Scenario: Multiple tool calls execute concurrently

- **WHEN** a step contains 3 tool calls each taking ~100ms and all tools are eligible for execution
- **THEN** `executeTools()` completes in approximately 100ms (wall-clock), not 300ms

#### Scenario: Single tool call executes normally

- **WHEN** a step contains exactly 1 eligible tool call
- **THEN** the tool executes and completes identically to the sequential path

#### Scenario: No eligible tool calls

- **WHEN** a step contains tool calls that are all provider-executed or have neither an Execute function nor a streaming execution function
- **THEN** `executeTools()` returns immediately without spawning any goroutines

#### Scenario: Streaming and single-result tools execute together

- **WHEN** a step contains one streaming tool and one single-result tool that are both eligible
- **THEN** both SHALL execute concurrently and finish independently

### Requirement: Independent error handling per tool

Each tool call SHALL handle errors independently. A failure in one tool's execution SHALL NOT cancel or affect the execution of other concurrent tool calls. Failed tools SHALL produce a `ToolResult` with `ModelOutput.Type` set to `ToolOutputErrorText` containing the error message, matching existing behavior.

#### Scenario: One tool fails while others succeed

- **WHEN** a step has 3 tool calls and the second tool returns an error
- **THEN** the first and third tools complete successfully
- **AND** the second tool produces a `ToolResult` with `ModelOutput.Type == ToolOutputErrorText`
- **AND** `StreamToolError` is emitted for the failed tool
- **AND** `StreamToolResult` is emitted for each successful tool

#### Scenario: All tools fail

- **WHEN** a step has multiple tool calls and all return errors
- **THEN** each tool produces its own `ToolResult` with `ToolOutputErrorText`
- **AND** a `StreamToolError` is emitted for each tool
- **AND** no `StreamToolResult` events are emitted

#### Scenario: ToModelOutput conversion failure handled independently

- **WHEN** a tool executes successfully but its `ToModelOutput` function returns an error
- **THEN** that tool produces a `ToolResult` with `ToolOutputErrorText`
- **AND** other concurrent tools are not affected

### Requirement: Context propagation to tool goroutines

The parent context passed to `executeTools()` SHALL be propagated to each tool goroutine's `Execute` call. When the parent context is cancelled, all running tool executions SHALL receive the cancellation signal.

#### Scenario: Parent context cancelled during execution

- **WHEN** the parent context is cancelled while tools are executing concurrently
- **THEN** each tool's `Execute` function receives a cancelled context
- **AND** tools that respect context cancellation terminate early

#### Scenario: Context values accessible in tool goroutines

- **WHEN** the parent context contains values (e.g., request-scoped data)
- **THEN** each tool goroutine can access those context values via the propagated context

### Requirement: Stream event emission from concurrent goroutines

Each tool goroutine SHALL report completion to the goroutine that started the tools, which SHALL emit StreamToolResult/StreamToolError via r.emit() as reports arrive. Model-step and approval-resumed events SHALL use non-deterministic completion order, not call order, matching upstream enqueue-on-promise-resolution. Tool goroutines SHALL NOT call r.emit() or OnChunk; OnChunk SHALL never be invoked concurrently.

#### Scenario: Events arrive in completion order

- **WHEN** tool A takes 200ms and tool B takes 50ms and both execute concurrently
- **THEN** `StreamToolResult` for tool B is emitted before `StreamToolResult` for tool A

#### Scenario: Error and success events interleave

- **WHEN** tool A fails after 50ms and tool B succeeds after 100ms
- **THEN** `StreamToolError` for tool A is emitted before `StreamToolResult` for tool B

#### Scenario: A fast result is delivered while a slow tool is still running

- **WHEN** tool A is still running and tool B in the same step has completed
- **THEN** a client reading the UI message stream receives tool B's output before tool A completes

#### Scenario: Emission is serialized

- **WHEN** several tools in a step complete at the same time
- **THEN** their events are emitted one at a time from the goroutine that started the tools
- **AND** `OnChunk` is never invoked concurrently

### Requirement: Tool results preserve call order

step.ToolResults SHALL follow original step.ToolCalls order regardless of completion or provider-executed, invalid, denied or executed origin, for deterministic next-step messages. Upstream uses arrival order; Go call order is recorded in test/conformance/upstream.yaml. Results for calls outside the step SHALL retain relative position ahead of step results; results for the same call SHALL retain their order.

#### Scenario: Results ordered by call position

- **WHEN** a step has tool calls [A, B, C] and they complete in order [B, C, A]
- **THEN** `step.ToolResults` contains results in order [A, B, C]

#### Scenario: Skipped tools do not occupy result slots

- **WHEN** a step has tool calls [A, B, C] where B is provider-executed
- **THEN** `step.ToolResults` contains results for [A, C] only, in that order

#### Scenario: Rejected and denied calls keep their call position

- **WHEN** a step has tool calls [A, B, C] where A names a tool missing from the tool set, B executes, and C is denied by an approval policy
- **THEN** `step.ToolResults` contains results in order [A, B, C], although A was handled while the model stream was read and C before any tool started

### Requirement: Callback invocation from concurrent goroutines

`OnToolCallStart` and `OnToolCallFinish` callbacks SHALL be invoked from within each tool's goroutine. Callbacks MAY be invoked concurrently from multiple goroutines. Callers providing these callbacks SHALL be responsible for their own goroutine safety.

#### Scenario: OnToolCallStart called before each tool executes

- **WHEN** a step has 3 concurrent tool calls and `OnToolCallStart` is set
- **THEN** `OnToolCallStart` is invoked 3 times, once per tool, before each tool's `Execute` call

#### Scenario: OnToolCallFinish called after each tool completes

- **WHEN** a step has 3 concurrent tool calls and `OnToolCallFinish` is set
- **THEN** `OnToolCallFinish` is invoked 3 times, once per tool, after each tool's execution completes or fails

#### Scenario: Callbacks invoked concurrently

- **WHEN** two tools complete at approximately the same time
- **THEN** their `OnToolCallFinish` callbacks MAY execute concurrently on different goroutines
