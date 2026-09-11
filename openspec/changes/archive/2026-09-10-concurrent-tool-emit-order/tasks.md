## 1. Emit from the tool goroutine

- [x] 1.1 Emit each tool's stream event from `executeSingleTool` via a deferred call, so it fires as the tool completes
- [x] 1.2 Reduce the post-`Wait` loop in `executeTools` to building `step.ToolResults` in tool call order
- [x] 1.3 Reduce the post-`Wait` loop on the approval-resume path likewise, keeping `approvedToolParts` in tool call order
- [x] 1.4 Correct the comment on `toolExecOutcome` that claims declaration-order emission matches upstream
- [x] 1.5 Correct the resume-path comment that also claimed declaration order
- [x] 1.6 Hold `emitMu` across the emit and the `OnChunk` callback, so the callback stays serial and in stream order

## 2. Conformance comparator

- [x] 2.1 Add `normalizeConcurrentToolOutputs` to sort each maximal run of adjacent `tool-output-available` chunks by `toolCallId`
- [x] 2.2 Apply it to both sequences at the top of `CompareChunks`
- [x] 2.3 Exclude `providerExecuted` chunks, whose order comes off the recorded wire, and let them end a run
- [x] 2.4 Include `tool-output-error`, which leaves the same goroutine as `tool-output-available`
- [x] 2.5 Add a table test covering empty input, runs at the slice end, mixed success and error runs, provider-executed splits, duplicate and missing `toolCallId`, and caller-slice immutability

## 3. Tests

- [x] 3.1 Flip `stream_events_arrive_in_declaration_order` to `stream_events_arrive_in_completion_order`, asserting the scenario at `concurrent-tool-execution/spec.md:72-75`
- [x] 3.2 Confirm `results_preserve_call_order` still passes, covering `spec.md:84`
- [x] 3.3 Confirm the comparator still catches a mispaired output value, a deleted output chunk, an output chunk moved across `finish-step`, and a reordering of non-output chunks

## 4. Verification

- [x] 4.1 `go test -short ./...` green
- [x] 4.2 Conformance suite green on anthropic, bedrock, openai and openai-compatible
- [x] 4.3 `mise run validate-parity-baseline` green
- [x] 4.4 `go test -race` green on the concurrent tool execution suite
- [x] 4.5 Repeat the anthropic `parallel-tool-calls` case 30 times to confirm the flake is gone
- [x] 4.6 Confirm a swap of two provider-executed outputs still fails, and that `OnChunk` order matches `FullStream` order over 200 runs
- [x] 4.7 Run `mise run parity-check` with `AI_SDK_UPSTREAM_ROOT` set to the pinned checkout
