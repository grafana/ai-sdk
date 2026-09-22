## 1. Implementation

- [x] 1.1 Emit tool events from the run goroutine as each tool completes, in `executeTools` and `resolveToolApprovals`
- [x] 1.2 Keep `step.ToolResults` and `approvedToolParts` in call order
- [x] 1.3 Normalize adjacent locally executed tool outputs in `CompareChunks`
- [x] 1.4 Update the `concurrent-tool-execution` emission requirement to the serialized contract

## 2. Verification

- [x] 2.1 `stream_events_arrive_in_completion_order` and the approval-resume test assert completion order
- [x] 2.2 `TestNormalizeConcurrentToolOutputs` covers sorting, provider-executed chunks and run boundaries
- [x] 2.3 `concurrent-tools.test.ts` reads the stream with `parseJsonEventStream` and `uiMessageChunkSchema`, and receives the fast tool's output while the slow tool is blocked on a gate
- [x] 2.4 Conformance suite, integration suite, `validate-parity-baseline` and `-race` pass
