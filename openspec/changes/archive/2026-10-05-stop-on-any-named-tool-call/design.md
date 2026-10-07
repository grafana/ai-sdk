## Context

Upstream `packages/ai/src/generate-text/stop-condition.ts` at `ai@7.0.116` defines `hasToolCall(...toolName)` as `steps[steps.length - 1]?.toolCalls?.some(toolCall => toolName.includes(toolCall.toolName)) ?? false`. Its tests cover a match in the last step, a match only in an earlier step, any of several names, none of several names, and no steps.

Go at `0e8f35e` had `HasToolCall(toolName string)` in `text.go`, evaluated against the latest `StepResult.ToolCalls`. `StreamText`, `GenerateText` and `ToolLoopAgent` all evaluate the same `StopCondition` values, so changing the helper changes all three consistently.

## Decisions

### 1. Variadic HasToolCall instead of a second helper

A variadic parameter keeps every one-name call valid and mirrors upstream's rest parameter. A separately named helper would leave two ways to express the same condition. The only incompatibility is storing the function in a `func(string) StopCondition` variable, which nothing in the repository does.

### 2. No names never stops

Upstream's `[].includes(...)` is always false, so an empty call never stops the loop. Go matches that with `slices.Contains` over an empty slice instead of treating zero names as an error, which would need a new error path on a function that returns a plain condition.
