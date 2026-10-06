## Context

`executeTools()` in `streamtext.go:713` iterates `step.ToolCalls` sequentially: each tool's `Execute` function blocks until completion before the next starts. The upstream TypeScript SDK uses `Promise.all` over all tool calls at the `model-call-end` event, executing them concurrently with no ordering guarantees.

The `fullStream` channel (`chan TextStreamPart`, buffer 256) is currently single-writer -- all writes go through `r.emit()` from the `run` goroutine. Go channels are safe for concurrent sends, so multiple goroutines can emit events without additional synchronization. However, `step.ToolResults` (a slice) requires mutex protection for concurrent appends.

Upstream reference: `create-execute-tools-transformation.ts` collects tool calls during stream processing, then at `model-call-end` runs `Promise.all(toolCallsToExecute.map(...))`. Each tool call independently handles its own errors (caught per-promise, emitted as `tool-error`). One tool failing does not cancel others.

## Goals / Non-Goals

**Goals:**
- Execute all tool calls within a step concurrently, completing in the time of the slowest tool
- Match upstream `Promise.all` semantics: no concurrency limit, no cancellation on individual tool failure
- Preserve error handling: tool failures produce `ToolOutputErrorText` results without affecting other tools
- Propagate parent context to all tool goroutines for external cancellation

**Non-Goals:**
- Per-tool timeouts (tracked separately in #17)
- Configurable concurrency limits
- Ordered event emission (upstream doesn't guarantee it either)
- Changes to `generateText` path directly. Note: `GenerateText` delegates to `StreamText` and inherits concurrent tool execution automatically, which matches upstream where both paths execute tools concurrently via `Promise.all`.

## Decisions

### Decision 1: goroutines + sync.WaitGroup over errgroup

**Choice**: Use goroutines with `sync.WaitGroup` from the stdlib.

**Alternatives considered**:
- `golang.org/x/sync/errgroup`: Provides automatic goroutine management and error collection. However, its `WithContext` variant cancels on first error (opposite of what we want), and using `new(errgroup.Group)` without context provides minimal benefit over WaitGroup while adding an external dependency. The project does not currently depend on `x/sync`.
- Raw goroutines without WaitGroup: No way to know when all tools complete.

**Rationale**: WaitGroup is idiomatic for "launch N goroutines, wait for all to finish" with no error aggregation needed. Each goroutine handles its own errors independently. No new dependency required.

### Decision 2: Pre-allocated result slots (no mutex needed)

**Choice**: Pre-allocate a `[]ToolResult` slice with one slot per executable tool call. Each goroutine writes to its assigned index. No mutex is needed because each goroutine writes to a distinct index.

**Alternatives considered**:
- `sync.Mutex`-protected `append`: Would work but introduces contention and does not preserve call order without additional sorting.

**Rationale**: Each goroutine owns exactly one slot, so there are no concurrent writes to the same memory. This eliminates mutex overhead and naturally preserves call order (see Decision 4). After `WaitGroup.Wait()`, the pre-allocated slice is appended to `step.ToolResults` sequentially.

Channel sends via `r.emit()` do NOT need a mutex -- Go channels are inherently safe for concurrent sends from multiple goroutines.

### Decision 3: Fire-as-complete event ordering

**Choice**: Emit `StreamToolResult`/`StreamToolError` events and invoke `OnToolCallStart`/`OnToolCallFinish` callbacks from within each goroutine as tools complete, in non-deterministic order.

**Alternatives considered**:
- Collect all results, then emit in original call order: Adds latency (must wait for slowest tool before emitting any results) and diverges from upstream where `Promise.all` callbacks resolve in completion order.
- Buffer events per-goroutine, emit sequentially after all complete: Same latency problem, unnecessary complexity.

**Rationale**: Matches upstream behavior. The TypeScript `controller.enqueue(result)` inside each promise resolves as that tool finishes, interleaving with other tools. Consumers already handle events by `ToolCallID`, not by position.

### Decision 4: ToolResults ordering -- preserve call order via pre-allocated slots

**Choice**: Pre-allocate `step.ToolResults` with a slot per executable tool call. Each goroutine writes to its assigned index. This avoids mutex contention on appends and preserves the original tool call order in the results slice.

**Rationale**: While event emission order doesn't matter, `step.ToolResults` is used to build the next step's messages. Preserving call order in the slice ensures deterministic message construction, matching upstream where `Promise.all` returns results in input order regardless of completion order.

### Decision 5: Callback concurrency is the caller's responsibility

**Choice**: `OnToolCallStart` and `OnToolCallFinish` callbacks may be invoked concurrently from multiple goroutines. This is documented but not synchronized internally.

**Rationale**: Matches upstream behavior where callbacks fire from within concurrent promises. Adding internal synchronization around user callbacks would serialize execution and negate the concurrency benefit. The callbacks are optional and callers who provide them should expect concurrent invocation.

## Risks / Trade-offs

- **[Non-deterministic event ordering]** -> Stream events arrive in completion order, not call order. Consumers already key on `ToolCallID`. This matches upstream. Documented in the change.

- **[Callback concurrency]** -> Users with non-goroutine-safe `OnToolCallStart`/`OnToolCallFinish` callbacks will see races. -> Mitigation: Document in the callback field's godoc that callbacks may be invoked concurrently.

- **[Goroutine overhead for single tool call]** -> When there's only one tool call (common case), we still spawn a goroutine + WaitGroup. -> Mitigation: Negligible overhead. Could optimize with a fast-path for len==1 but YAGNI -- the goroutine cost is ~2KB stack, insignificant vs network I/O in tool execution.

- **[No concurrency limit]** -> A step with many tool calls spawns that many goroutines. -> Mitigation: Matches upstream (no limit). Tool counts per step are typically small (1-5). If this becomes a problem, a semaphore can be added later without API changes.
