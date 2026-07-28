## Why

The upstream Vercel AI SDK executes all tool calls within a single step concurrently via `Promise.all`, finishing in the time of the slowest tool. The Go port executes them sequentially in a plain `for` loop (`streamtext.go:713`), taking the sum of all tool execution times. For N tool calls at T seconds each, upstream completes in ~T seconds while the Go port takes ~N*T seconds. This is the most significant behavioral divergence in the tool execution path.

## What Changes

- Replace sequential tool execution in `executeTools()` with concurrent execution using `errgroup` (or goroutines + WaitGroup)
- Each tool call runs in its own goroutine within the step, results collected and emitted once all complete
- Add synchronization around stream event emission since `fullStream` is currently single-writer with no concurrent access
- Preserve existing error handling semantics: one tool failing produces a `ToolOutputErrorText` result without cancelling other tools
- Pass parent context to all tool goroutines for cancellation propagation

## Capabilities

### New Capabilities
- `concurrent-tool-execution`: Concurrent execution of multiple tool calls within a single step, matching upstream `Promise.all` semantics

### Modified Capabilities

## Impact

- `streamtext.go`: `executeTools()` function rewritten from sequential loop to concurrent goroutine-based execution
- `tool.go`: No changes to tool types/interfaces -- `ToolExecuteFunc` signature remains the same
- `textstream.go`: No changes to stream part types
- Stream event ordering: `StreamToolResult`/`StreamToolError` events will arrive in completion order rather than call order (matches upstream behavior)
- Callback ordering: `OnToolCallStart`/`OnToolCallFinish` will fire in non-deterministic order
- No new dependencies: `errgroup` is in the Go stdlib (`golang.org/x/sync/errgroup`) or can use goroutines + `sync.WaitGroup`
- No breaking API changes: all public types and function signatures remain unchanged
