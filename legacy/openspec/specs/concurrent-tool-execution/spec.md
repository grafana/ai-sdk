## Purpose

Concurrent execution of multiple tool calls within a single step, matching upstream Vercel AI SDK `Promise.all` semantics. Tools execute in parallel via goroutines, completing in the time of the slowest tool rather than the sum of all tool execution times.

## Requirements

### Requirement: Concurrent tool execution within a step

When a model returns multiple tool calls in a single step, `executeTools()` SHALL execute all eligible tool calls concurrently using goroutines. Each tool call SHALL run in its own goroutine. The function SHALL wait for all goroutines to complete before returning.

Eligible tool calls are those where `ProviderExecuted` is false AND the tool exists in `params.Tools` with a non-nil `Execute` function.

#### Scenario: Multiple tool calls execute concurrently

- **WHEN** a step contains 3 tool calls each taking ~100ms and all tools are eligible for execution
- **THEN** `executeTools()` completes in approximately 100ms (wall-clock), not 300ms

#### Scenario: Single tool call executes normally

- **WHEN** a step contains exactly 1 eligible tool call
- **THEN** the tool executes and completes identically to the sequential path

#### Scenario: No eligible tool calls

- **WHEN** a step contains tool calls that are all provider-executed or have no Execute function
- **THEN** `executeTools()` returns immediately without spawning any goroutines

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

Each tool goroutine SHALL emit `StreamToolResult` or `StreamToolError` events via `r.emit()` as the tool completes. Events SHALL be emitted in completion order (non-deterministic), not in the original tool call order. This matches upstream behavior where `Promise.all` resolves in completion order.

#### Scenario: Events arrive in completion order

- **WHEN** tool A takes 200ms and tool B takes 50ms and both execute concurrently
- **THEN** `StreamToolResult` for tool B is emitted before `StreamToolResult` for tool A

#### Scenario: Error and success events interleave

- **WHEN** tool A fails after 50ms and tool B succeeds after 100ms
- **THEN** `StreamToolError` for tool A is emitted before `StreamToolResult` for tool B

### Requirement: Tool results preserve call order

The `step.ToolResults` slice SHALL contain results in the same order as the original `step.ToolCalls` entries, regardless of completion order. This ensures deterministic message construction for subsequent steps.

#### Scenario: Results ordered by call position

- **WHEN** a step has tool calls [A, B, C] and they complete in order [B, C, A]
- **THEN** `step.ToolResults` contains results in order [A, B, C]

#### Scenario: Skipped tools do not occupy result slots

- **WHEN** a step has tool calls [A, B, C] where B is provider-executed
- **THEN** `step.ToolResults` contains results for [A, C] only, in that order

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
