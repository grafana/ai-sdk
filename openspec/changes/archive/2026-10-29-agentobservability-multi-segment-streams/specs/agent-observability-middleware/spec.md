## ADDED Requirements

### Requirement: Stream recording spans every response segment
A streaming recording SHALL produce one generation per call and observe every part until upstream closes or the request context ends, including parts after any `PartFinish`. A `PartFinish` SHALL NOT end the recorder. On upstream close before the request context ends, the completion time SHALL be the stream end. The first-token time SHALL remain the first semantic output of the stream. Every part SHALL still pass through unchanged and in order.

#### Scenario: Multi-segment stream closes normally
- **GIVEN** a stream carries two response segments, each starting with response metadata and ending with a `PartFinish` that reports its own usage and finish reason
- **WHEN** upstream closes after the second finish
- **THEN** exactly one generation SHALL be recorded
- **AND** it SHALL carry the second finish's reason, the second segment's usage, and output from both segments in observed order
- **AND** its completion time SHALL NOT precede the observation of the second finish, and its first-token time SHALL be the first segment's first semantic output

#### Scenario: Single-finish stream with nothing after the finish
- **WHEN** a stream with one `PartFinish` closes normally and no part follows the finish
- **THEN** the recorded generation SHALL carry the output, usage and finish reason observed through that finish
- **AND** its call error SHALL be the latest `PartError` observed before the finish, if any

#### Scenario: Provider error after the only finish, then close
- **WHEN** a stream emits one `PartFinish`, then a `PartError`, and upstream closes normally
- **THEN** the generation SHALL carry that stream error as its call error

#### Scenario: Last non-empty response identity wins
- **WHEN** each segment's response metadata reports a different response ID and response model
- **THEN** the generation SHALL carry the last non-empty response ID and response model observed

### Requirement: Context end after a finish records the finished segments
When the request context ends after at least one `PartFinish` was observed, cancellation or timeout SHALL NOT be the call error; the latest `PartError` carrying an error observed, before or after the last finish, SHALL be. With no `PartError` after the last finish, the recorder SHALL record the state captured at that finish, completed when that finish was observed; otherwise it SHALL record the state so far. An upstream close observed after the context ended SHALL count as a context end.

#### Scenario: Consumer stops at finish and cancels
- **GIVEN** a stream with no `PartError` whose upstream stays open after a `PartFinish`
- **WHEN** the consumer reads that finish, stops reading and cancels the request context
- **THEN** the generation SHALL be recorded once with the finished segment's output, usage and finish reason
- **AND** it SHALL carry no call error

#### Scenario: Stall between segments
- **GIVEN** a stream emits one finished segment and then stalls until its context deadline or cancellation
- **WHEN** the context ends
- **THEN** the generation SHALL carry the first segment's usage and finish reason without a call error
- **AND** its completion time SHALL be when the first finish was observed, not when the context ended

#### Scenario: Provider error after a finish
- **WHEN** a stream emits a finished segment, then a `PartError`, and the context ends before upstream closes
- **THEN** the generation SHALL carry that stream error as its call error, not the cancellation

#### Scenario: Provider error before a finish, then cancel
- **GIVEN** a stream emits a `PartError`, then a `PartFinish`, and its upstream stays open
- **WHEN** the context ends before upstream closes
- **THEN** the generation SHALL carry that stream error as its call error, not the cancellation
- **AND** it SHALL carry the usage and finish reason captured at that finish

#### Scenario: Upstream closes after the context ended
- **WHEN** a stream emits a `PartFinish` and upstream closes only after the request context has ended
- **THEN** the generation SHALL carry the state captured at that finish and no call error

#### Scenario: Consumer stops reading without canceling
- **WHEN** a consumer stops reading after a `PartFinish` without canceling the context
- **THEN** the recorder SHALL NOT end at that finish
- **AND** it SHALL end once upstream closes and the remaining parts have been handed to the bounded downstream channel, or when the request context ends, whichever comes first

## MODIFIED Requirements

### Requirement: StreamRecorder accumulates streamed generation state

`StreamRecorder.Observe` SHALL accumulate `Generation.Output` from provider stream parts. `Generation()` SHALL contain an assistant message for accumulated model parts and tool-role messages for completed results. Assistant text, reasoning, tool-call, and media parts SHALL retain first-observed provider-event order. Finish reason SHALL come from the last `PartFinish`; every usage-bearing part SHALL use shared streaming aggregation.

#### Scenario: Stream usage preserves strongest values

- **GIVEN** usage is split across multiple stream parts
- **AND** the finish part omits some normalized counters
- **WHEN** the recorder produces a generation
- **THEN** its usage SHALL contain the counters the finish reports and, for the counters it omits, the strongest values observed earlier in its response segment, as supported by the Agent Observability usage schema

#### Scenario: Multi-finish usage follows the last finish

- **GIVEN** a stream emits several `PartFinish` parts, each reporting usage
- **WHEN** the recorder produces a generation
- **THEN** its usage SHALL be the last finish's segment usage under shared streaming usage aggregation
- **AND** counters SHALL NOT be summed or take the greatest value across segments

#### Scenario: Later finish reason wins

- **WHEN** a stream emits a `PartFinish` with tool calls and later a `PartFinish` with stop
- **THEN** the generation's stop reason SHALL map the later finish reason

#### Scenario: Reused text and reasoning IDs start new parts after a finish

- **GIVEN** one segment emits text and reasoning with ID `"0"` and ends with `PartFinish`
- **WHEN** the next segment emits text and reasoning that reuse ID `"0"`
- **THEN** the assistant output SHALL contain the first segment's parts followed by separate parts for the second segment
- **AND** the second segment's deltas SHALL NOT be appended to the first segment's parts

#### Scenario: Reasoning text accumulates across deltas

- **GIVEN** a stream that emits three `PartReasoningDelta` events with text fragments `"I "`, `"think "`, `"so"`
- **WHEN** `StreamRecorder.Observe` is called for each
- **AND** `StreamRecorder.Generation()` is called at end-of-stream
- **THEN** the resulting reasoning part in `Generation.Output` SHALL contain the concatenated reasoning text `"I think so"`

#### Scenario: Signature-only reasoning is omitted

- **GIVEN** a stream emits a reasoning block containing an Anthropic signature but no reasoning text
- **WHEN** `StreamRecorder.Generation()` is called
- **THEN** `Generation.Output` SHALL NOT contain an empty thinking part

#### Scenario: Text deltas concatenate

- **GIVEN** a stream emits `PartTextDelta{Text: "Hello, "}` then `PartTextDelta{Text: "world"}`
- **WHEN** the recorder is observed for each
- **THEN** the resulting assistant message's text part SHALL equal `"Hello, world"`

#### Scenario: Tool-call deltas accumulate by tool-call ID

- **GIVEN** a stream emits multiple `PartToolInputDelta` events for the same tool-call ID with incremental JSON argument fragments
- **WHEN** the recorder is observed
- **THEN** the resulting tool-call part SHALL have the concatenated argument JSON

#### Scenario: Streamed media preserves observed assistant-part order

- **GIVEN** a stream interleaves supported file, text, reasoning, tool-call, and reasoning-file events
- **WHEN** the recorder observes the stream and produces a generation
- **THEN** the assistant output parts SHALL follow the order in which each part was first observed
- **AND** the first supported file event SHALL set `FirstChunkAt()` when no earlier payload-bearing event was observed

#### Scenario: Deltas and supported files accumulate into assistant parts
- **WHEN** `Observe` receives text, reasoning, tool-input, and supported file events
- **THEN** `PartTextDelta` SHALL append to the active assistant text part, non-empty `PartReasoningDelta` to reasoning, and `PartToolInputDelta` to the active tool-call part
- **AND** signature-only reasoning without visible text SHALL NOT produce a thinking part
- **AND** supported `PartFile` and `PartReasoningFile` SHALL map to media parts

#### Scenario: Preliminary tool results coalesce without collapsing completed results
- **WHEN** a stream emits tool results
- **THEN** preliminary updates SHALL coalesce by exact tool-call ID and tool name
- **AND** only completed tool results SHALL enter final output
- **AND** distinct completed results SHALL remain distinct even when an ID is reused

### Requirement: RecordingMiddleware wraps generate and stream

`RecordingMiddleware(opts)` SHALL return `middleware.Middleware` with generate and stream hooks. A nil resolved client SHALL pass through unchanged. Active calls SHALL build `GenerationStart` using `BuildGenerationStart(ctx, model.Provider(), model.ModelID(), opts.ContextProvider(ctx))`, call `client.StartGeneration` or `client.StartStreamingGeneration` respectively, then invoke the inner model. Params and results SHALL NOT be modified.

#### Scenario: Generate path records on success

- **GIVEN** a `RecordingMiddleware` with a non-nil `ClientResolver`
- **WHEN** the inner model's `DoGenerate` returns a non-nil result and `nil` error
- **THEN** the middleware SHALL call `StartGeneration` once
- **AND** SHALL call `recorder.SetResult` once with the generation mapped from `params`, `result`, `ctxInfo`, and the original `GenerationStart`
- **AND** SHALL NOT call `recorder.SetCallError`

#### Scenario: Generate path records on error

- **GIVEN** a `RecordingMiddleware` with a non-nil `ClientResolver`
- **WHEN** the inner model's `DoGenerate` returns a non-nil error
- **THEN** the middleware SHALL call `recorder.SetCallError(err)` once
- **AND** the same error SHALL be returned to the caller

#### Scenario: Stream error records partial generation usage

- **GIVEN** a stream reports usage and then emits `PartError`
- **WHEN** the recording goroutine finalizes
- **THEN** it SHALL call `recorder.SetCallError` with the stream error
- **AND** it SHALL call `recorder.SetResult` with the partial generation and aggregated usage

#### Scenario: Stream path records at end of stream

- **GIVEN** a `RecordingMiddleware` with a non-nil `ClientResolver`
- **WHEN** the inner model's `DoStream` returns a result stream that closes normally after N parts
- **THEN** the middleware SHALL call `StartStreamingGeneration` once
- **AND** the consumer SHALL receive exactly the same N parts in the same order
- **AND** `recorder.SetResult` SHALL be called once after the upstream channel closes

#### Scenario: Stream cancellation records an error and cleans up

- **WHEN** the consumer cancels its context before the upstream completes and before any `PartFinish` was observed
- **THEN** the recording goroutine SHALL NOT block indefinitely
- **AND** the generation SHALL record the context cancellation as its call error
- **AND** the middleware SHALL NOT start a detached goroutine that waits indefinitely for the upstream channel to close

#### Scenario: Successful calls finalize the corresponding recorder
- **WHEN** an active generate or stream call succeeds
- **THEN** generate SHALL use `client.StartGeneration`, map the result with the original `GenerationStart` for requested-model fallback, and call `recorder.SetResult`
- **AND** stream SHALL use `client.StartStreamingGeneration`, tee each result part into `StreamRecorder`, and call `recorder.SetResult` once at finalization with the stream generation selected by the finalization rules

#### Scenario: Opening errors and usage on error parts are recorded
- **WHEN** a call returns an error before a stream opens or a stream emits `PartError`
- **THEN** an opening error SHALL call `recorder.SetCallError(err)`
- **AND** a `PartError` SHALL call `recorder.SetCallError(err)` and `recorder.SetResult` with partial generation and aggregated usage observed before or on the error part

### Requirement: Fail-open local recording diagnostics
Agent Observability recording options SHALL provide an optional record-error handler invoked after finalization when local validation or enqueueing fails and an optional completion handler invoked once after every started recorder ends. Handler panics SHALL be recovered, and neither recorder failure nor handler failure SHALL alter the model call result or stream. Streaming recorders SHALL end before downstream EOF while completion/error callbacks SHALL NOT delay that EOF.

#### Scenario: Queue failure is reported without failing traffic
- **WHEN** a recorder cannot enqueue a finalized generation
- **THEN** the handler SHALL receive the local error once
- **AND** the original model response SHALL remain unchanged even if the handler panics

#### Scenario: Process owner tracks an abandoned stream
- **WHEN** a stream consumer stops reading and request cancellation ends its recorder asynchronously
- **THEN** the completion handler SHALL run exactly once after the generation is finalized
- **AND** a blocking completion or error handler SHALL NOT delay downstream EOF

#### Scenario: Delivery of a finish does not imply the generation is enqueued
- **WHEN** a consumer receives a `PartFinish` from a recorded stream
- **THEN** the generation SHALL NOT be guaranteed to be enqueued at that point
- **AND** it SHALL be enqueued before downstream EOF and before the completion handler runs, so a process owner that closes the shared client SHALL first drain the stream to EOF or wait for the completion handler

### Requirement: Recording cancellation precedence and producer ownership

The recording goroutine SHALL select on `ctx.Done()` to avoid blocking on consumer disconnect. Cancellation before observed upstream completion and before any observed `PartFinish` SHALL be the call error, taking precedence over an earlier `PartError`. The middleware SHALL NOT start an unbounded detached drain if the provider ignores cancellation; provider producers SHALL be responsible for honoring the call context.

#### Scenario: Cancellation supersedes an earlier stream error
- **WHEN** a stream emits `PartError` and the context cancels before observed upstream completion and before any `PartFinish`
- **THEN** recording SHALL use cancellation as the call error
- **AND** the recording goroutine SHALL avoid blocked downstream sends without starting an unbounded detached drain

#### Scenario: Cancellation after a finish is not the call error
- **WHEN** the context cancels after a `PartFinish` was observed
- **THEN** recording SHALL NOT use cancellation as the call error
- **AND** the latest `PartError` observed, if any, SHALL remain the call error
- **AND** the recording goroutine SHALL still avoid blocked downstream sends without starting an unbounded detached drain
