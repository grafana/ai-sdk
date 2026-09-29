## MODIFIED Requirements

### Requirement: Concurrent tool execution within a step

When a model returns multiple tool calls in a single step, `executeTools()` SHALL execute all eligible tool calls concurrently using goroutines. Each tool call SHALL run in its own goroutine. The function SHALL wait for all goroutines to complete before returning.

Eligible tool calls are those where `ProviderExecuted` is false AND the tool exists in the effective step execution tool set with either a non-nil `Execute` function or a configured streaming execution function. The existing allowed-finish and approval gates SHALL continue to apply.

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
