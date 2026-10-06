## 1. Core concurrent execution

- [x] 1.1 Refactor `executeTools()` in `streamtext.go` to launch a goroutine per eligible tool call using `sync.WaitGroup`, replacing the sequential `for` loop
- [x] 1.2 Pre-allocate `step.ToolResults` slots indexed by position within the filtered executable tool calls, so each goroutine writes to its assigned index without mutex contention
- [x] 1.3 Propagate the parent `ctx` to each goroutine's `tool.Execute()` call
- [x] 1.4 Ensure `OnToolCallStart` is called before `Execute` and `OnToolCallFinish` after completion/failure within each goroutine, preserving per-tool callback semantics

## 2. Event emission and result ordering

- [x] 2.1 Emit `StreamToolResult`/`StreamToolError` via `r.emit()` from within each goroutine as tools complete (fire-as-complete ordering)
- [x] 2.2 Call `r.callOnChunk()` from within each goroutine alongside `r.emit()` for chunk callback consistency
- [x] 2.3 After `WaitGroup.Wait()`, compact the pre-allocated results slice to remove nil slots (skipped provider-executed/external tools) and assign to `step.ToolResults`

## 3. Error handling

- [x] 3.1 Preserve independent error handling: each goroutine catches its own `Execute` error and produces a `ToolOutputErrorText` result without affecting other goroutines
- [x] 3.2 Preserve `ToModelOutput` conversion error handling within each goroutine, producing `ToolOutputErrorText` on failure

## 4. Tests

- [x] 4.1 Add a test verifying concurrent execution: multiple tools with artificial delays complete in ~max(delays) wall-clock time, not sum(delays)
- [x] 4.2 Add a test verifying independent error handling: one tool fails while others succeed, all produce correct results
- [x] 4.3 Add a test verifying `step.ToolResults` ordering matches original `step.ToolCalls` order regardless of completion order
- [x] 4.4 Add a test verifying context cancellation propagates to all concurrent tool goroutines
- [x] 4.5 Add a test verifying the single-tool-call path still works correctly
- [x] 4.6 Run full test suite (`make test`) and verify no regressions
