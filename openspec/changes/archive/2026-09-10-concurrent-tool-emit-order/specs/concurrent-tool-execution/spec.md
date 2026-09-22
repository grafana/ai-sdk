## MODIFIED Requirements

### Requirement: Stream event emission from concurrent goroutines

Each tool goroutine SHALL report its completion to the goroutine that started the tools, and that goroutine SHALL emit the tool's `StreamToolResult` or `StreamToolError` event via `r.emit()` as the report arrives. Events SHALL be emitted in completion order (non-deterministic), not in the original tool call order, for both a model step's tool calls and tools resumed after approval. Tool goroutines SHALL NOT call `r.emit()` or `OnChunk` themselves, so `OnChunk` is never invoked concurrently. This matches upstream behavior where each tool's result is enqueued as its promise resolves.

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
