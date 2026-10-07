## Why

[Issue #298](https://github.com/grafana/ai-sdk/issues/298): Go `HasToolCall(toolName string)` stops only for one named tool, while the registered `ai@7.0.116` `hasToolCall(...toolName)` accepts several names and stops when the latest step called any of them. Go callers had to compose their own condition to express a set of tool names.

## What Changes

- `HasToolCall` becomes variadic: `HasToolCall(toolNames ...string)`. It stops when the most recent step called any of the names, ignores calls in earlier steps, and never stops when given no names, all matching upstream.
- Every existing one-name call compiles and behaves as before.
- The agent loop guide shows the multi-name form.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `functional-options`: Add the `HasToolCall` stop condition contract alongside `WithStopWhen`.

## Impact

`text.go`, its tests and one guide paragraph. The change is source compatible for every call site; only code that stores `HasToolCall` in a variable of type `func(string) StopCondition` would need updating, and none exists in this repository.
