## Why

`openspec/specs/concurrent-tool-execution/spec.md` requires each tool's `StreamToolResult` or `StreamToolError` to be emitted as that tool completes. `executeTools` and `resolveToolApprovals` instead waited for every tool and emitted in call order, so no result reached the stream until the slowest tool finished. Upstream `ai@7.0.65` emits each result as its tool resolves. See #161.

## What Changes

- Tool goroutines report completion on a buffered channel, and the run goroutine emits each result as its report arrives. `step.ToolResults` and the approval-resume `approvedToolParts` are still built in call order afterwards.
- `CompareChunks` sorts each run of adjacent `tool-output-available` and `tool-output-error` chunks by `toolCallId`. Recorded fixtures use tools that finish instantly, so their relative order depends on the scheduler. Chunks with `providerExecuted: true` and outputs of calls rejected with `tool-input-error` stay in place.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `concurrent-tool-execution`: tool goroutines report completion and the goroutine that started them emits each event, so emission is serialized while still following completion order.
- `conformance-testing`: adjacent locally executed tool outputs are compared without regard to their order.

## Impact

- `streamtext.go`: tool events are emitted as each tool completes, at both call sites. All events are still emitted from the run goroutine, so `OnChunk` is never called concurrently.
- `test/conformance/runner.go`: `CompareChunks` normalizes before comparing.
- `test/integration`: a `concurrent-tools` scenario and client test prove the ordering through the registered TypeScript client.
- Final text, tool outputs, `step.ToolResults`, provider requests and turn duration are unchanged.
- Sibling tool outputs that finish at the same moment can arrive in either order, where upstream is deterministic. Recorded in `upstream.yaml`.
