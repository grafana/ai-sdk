# Design

## Context

See proposal.md (Why). Current code:

- **The stream tee ends early.** `recording.go` `runStreamTee` observes parts only while `finished` is false. On the first `PartFinish` it calls `finalize(false)`, which runs `SetFirstTokenAt`, `SetResult(filterGeneration(...))` and `End`, and it does so before the part is forwarded (`tee <- part`).
  - The deferred comment explains why. This ordering came from #438: at the time, the Gateway handler canceled right after writing `finish`. That turned successful streams into `call_error: timeout`, or lost them when the client shut down first.
  - Cancellation handling: `ctx.Done()` sets `canceled`, an optional `drainStreamUntil` runs, and `finalize(true)` calls `SetCallError(ctx.Err())`.
- **`StreamRecorder` accumulates state with these rules** (`map_stream.go`):
  - `usage` calls `streamusage.Aggregator.Observe` on every part. Today it keeps the strongest value per counter across the whole stream, and Raw is last non-empty. Logger and prometheus use the same aggregator.
  - `finishReason` is overwritten on each finish, so the last one wins.
  - `responseID`, `responseModel` and `response` are overwritten by each non-empty `PartResponseMeta` field, so the last non-empty value wins.
  - Text and reasoning accumulators live in `texts`/`reasonings` maps keyed by part ID. `outputOrder` refers to them by ID, and `buildOutput` looks them up again.
  - Tool calls are keyed by tool-call ID through `activeToolCalls`/`toolResultByIdentity`.
- **agento11y v0.19.0 recorder API** (`agento11y/client.go`):
  - `SetResult` and `SetFirstTokenAt` only store values and can be called repeatedly. `SetCallError` cannot be cleared.
  - `End` is idempotent. It stamps `completedAt` only when `Generation.CompletedAt` is zero (`normalizeGeneration`), and the span end and duration metric use the normalized `CompletedAt`.
  - There is no update-after-End and no multi-response concept.
- **How downstream consumers finish reading:**
  - The Gateway reads past each finish until the provider closes (`providerwire/v4/stream.go` `PartFinish` → `streamPartContinue`). `PartError` also continues.
  - `AgentObservabilityRuntime.Close` waits for recordings through `OnRecordComplete` (`model_observability.go`, `recordings` WaitGroup, `recordingComplete`).
  - Core `StreamText` reads the provider stream until it closes (`streamtext.go` step loop).
- **Anthropic usage carries over between messages:**
  - The Anthropic stream adapter keeps the previous message's output and reasoning counts at `message_start` (`providers/anthropic/convert_usage.go` `resetUsage`), and so does upstream.
  - The Go adapter also puts that usage on `PartResponseMeta` (`convert_stream.go` `message_start`).
  - In upstream `programmatic-tool-calling/expected-provider-parts.jsonl`, finishes 2–14 each report `outputTokens.total` 725, while their raw usage is 0.

## Goals / Non-Goals

**Goals:**
- Match upstream `streamText` usage selection for every stream-summary middleware.
- Keep the existing guarantees: the recorder ends before downstream EOF, callbacks never delay EOF, the generation filter runs once per generation, and cancellation cleanup stays bounded.

**Non-Goals:**
- Exact per-message usage totals. Upstream reports the last finish's usage, and summing would deviate from it.
- Changes to providers or conformance fixtures.
- Per-segment generations, or new agento11y concepts.

## Decisions

### D1. Hybrid finalization: observe every part, end at close or context end

`runStreamTee` observes every part. A `PartFinish` no longer ends the recorder.

At each finish, after `Observe`, the tee goroutine captures a snapshot of stream-derived state only, through an unexported `StreamRecorder` method:
- the built output (the `buildOutput` result at that finish). Watermarks are not used: tool results and tool-call accumulators are mutated in place after a finish (`map_stream.go` `r.toolResults[index] = result`, `finishToolCall`, `matchToolCall`), so they cannot reproduce the state at the finish;
- usage, finish reason, response identity and `CallError()`;
- `FirstChunkAt()` and the finish observation time.

It also clears an "error since finish" flag. The flag is set by the same condition that updates `callError`: a `PartError` with a non-nil `APICallError` (`map_stream.go` `Observe`).

Prompt-derived fields are composed once, in `finalize`: system, input, controls, tools and provider-option metadata. This matters because `Generation()` re-converts the prompt and base64-encodes media on every call (`map_stream.go` `Generation`, `media.go` `inlineMediaURL`). Snapshot capture runs under the same `streamrecover.Recover` guard as `finalize` (`recording.go`). If it panics, the snapshot is marked invalid, and `finalize` falls back to the current state while still applying the call-error rule: `streamRec.CallError()`, never `ctx.Err()` after a finish. The guard is defensive: after D4's per-segment refs it has no reachable trigger, and no test covers it.

**Call-error rule.**
- After at least one `PartFinish`, cancellation or timeout is never the generation's call error.
- The call error is the latest provider `PartError` observed (`streamRec.CallError()`), whether it came before or after the last finish.
- The recorded state is the last-finish snapshot when no `PartError` followed that finish. Otherwise it is the current state.
- Before any finish, cancellation stays the call error (unchanged).

`finalize` runs exactly once and picks one of four outcomes. "Context ends" includes an upstream close observed after the context already ended (`canceled = ctx.Err() != nil`).

| Finalization cause | Recorded generation | Call error |
|---|---|---|
| Upstream closes and the context has not ended | Current state, including a partial trailing segment after the last finish, with the last finish's reason | `streamRec.CallError()` if any |
| Context ends, a finish was seen, no `PartError` since that finish | Last snapshot: `CompletedAt` set to that finish's time, `FirstTokenAt` from the snapshot's `FirstChunkAt` | `streamRec.CallError()` if any (a `PartError` before the finish) |
| Context ends, a finish was seen, `PartError` since that finish | Current state | `streamRec.CallError()` (that `PartError`) |
| Context ends before any finish | Current state (unchanged) | `ctx.Err()` (unchanged) |

In every case `finalize` filters once (`filterGeneration`), calls `SetResult` once, then `endRecording` before `close(tee)`.

The snapshot is kept in the tee goroutine rather than pushed to the recorder with repeated `SetResult`. The outcome is the same, because the recorder exposes nothing before `End`. Keeping it local preserves "filter invoked once per generation", runs the consumer filter (and its panic recovery) once, and keeps `SetCallError`, which cannot be cleared, out of provisional states.

On the close paths, `FirstTokenAt` still comes from `streamRec.FirstChunkAt()`, which is set once, at the first segment's first output. The snapshot path uses the snapshot's `FirstChunkAt`, so time to first token never exceeds the duration.

The snapshot's `CompletedAt` keeps a stall or a late cancel from inflating duration. A consumer `GenerationFilter` that builds a fresh `Generation` drops it, and agento11y then falls back to `End` time.

Rejected alternatives:
- *Keep finalizing at the first finish.* Usage, finish reason, timing and output stay wrong for multi-segment streams.
- *Hold each finish until the next part or close.* This adds latency to the last finish and stalls when the provider idles.
- *An opt-in `RecordingOptions` switch.* It splits behavior for little benefit (YAGNI). The module is alpha, so the contract change is acceptable.
- *Cancellation as call error after a finish.* This extends the pre-finish precedence rule. It is rejected because a finished segment is completed model output. The #438 pattern (stop at finish, then cancel) and a Gateway stall after a finish would again export `call_error: timeout` or cancellation for output the model completed. Logs and metrics keep the request-level classification.

### D2. Usage: the last finish's segment usage, matching upstream

Upstream picks the last finish's usage. Each Anthropic `message_stop` becomes a `finish` (`@ai-sdk/anthropic@4.0.82`). `streamText` turns every `finish` into a `model-call-end` and overwrites the step usage, finish reason and provider metadata (`ai@7.0.143` `stream-text.ts`, `model-call-end`). The OTel integration overwrites its inference-span usage attributes at each `onLanguageModelCallEnd` (`packages/otel/src/open-telemetry.ts`). Go core `StreamText` already overwrites `step.Usage` on each `PartFinish` (`streamtext.go`).

`internal/streamusage.Aggregator` adopts that rule for every stream-summary middleware (logger, prometheus, Agent Observability):
- a `PartFinish` closes a response segment;
- the closed segment takes each counter from the finish when the finish reports it, and otherwise the greatest value observed in that segment;
- once a finish was observed, the aggregate is the last closed segment's usage, and usage on parts after it does not change the aggregate;
- before any finish (a stream that ends on an error or cancellation), each counter keeps its greatest observed value, as today;
- Raw follows the same selection, with the latest non-empty payload in place of the greatest value.

The segment fallback keeps #25's protection for finishes that omit counters. It also ignores Go Anthropic's `PartResponseMeta` usage, which repeats the previous message's output count (`convert_usage.go` `resetUsage`) and has no upstream counterpart, whenever the finish reports that counter.

Logs, metrics and the generation share the aggregator, so they report the same usage on every path. That includes a stall after a finish, where the generation's last-finish snapshot and the logs' aggregate both stop at that finish.

This reverses #25's scenario "Provisional terminal usage is lower": a lower value on the finish now wins, as in upstream. No in-repo provider reports a lower finish counter than an earlier part of the same response. Only Anthropic reports usage outside `finish`, and its finish carries the full usage.

On `programmatic-tool-calling` the aggregate is input 4551 and output 197, upstream's step usage. The provider bills 7920 and 922 across the 15 messages, so upstream, and now this module, understate multi-message totals. Exact per-message totals would deviate from upstream and are not pursued.

Rejected alternatives:
- *Keep the greatest value per counter across the stream.* It gives 4551 and 725, mixing segments, and matches neither upstream nor core `StreamText`.
- *Segment max-then-sum.* It gives 725×14+197 = 10347 output tokens. The `message_start` carry-over also double counts within a segment: in `tool-search-regex`, a carried 163 plus 67 gives 326 against 230.
- *Sum each segment's finish usage.* This still gives 10347 on the programmatic fixture, and upstream does not sum.
- *Change only the Agent Observability generation.* Logs and metrics would then disagree with the generation on multi-message streams.

### D3. Last finish reason and last non-empty identity win

These rules are already how `Observe` behaves (see Context): finish reason, response ID, response model and complete response identity come from the last non-empty values. They agree with the last finish reason, and need no code change.

Raw usage follows D2: the last finish's non-empty raw payload, or otherwise the latest in that segment. So the `server_tool_use` metadata that `metadataFromUsage` derives from Raw reflects the last message, and Anthropic raw resets per message. The Gateway drops raw-usage metadata in `filterAgentGeneration`.

The other provider metadata is not recorded by `StreamRecorder`, so it needs no rule. `hooks.go` has no stream-finish logic, and the generation filter receives the recorded state's finish reason, the latest one.

### D4. Text and reasoning accumulators restart at each finish

On `PartFinish`, `StreamRecorder` replaces its `texts` and `reasonings` maps with empty ones. `streamOutputRef` keeps a pointer to its text or reasoning accumulator, so `buildOutput` no longer looks up either map by ID (today it reads `r.texts[ref.id]` and `r.reasonings[ref.id]`). A reused ID in the next segment then creates a new part, appended in observed order.

Tool calls and tool results stay keyed for the whole stream. Tool and approval IDs are unique across the stream, and a provider-executed result can arrive in a later segment: `programmatic-tool-calling` has its call in segment 1 and its result in segment 15. The Gateway runtime applies the same rule (`docs/concepts/gateway-glossary.md` "Response segment").

Output remains one assistant message, with every segment's parts in order, plus tool-role messages. agento11y has one generation per recorder.

Rejected alternatives:
- *Merge by ID.* It concatenates segment 2's text into segment 1's part.
- *Key by segment index.* It works the same, but needs a wider key in every lookup.

### D5. Timing contract change and consumer behavior

The recorder ends at upstream close or context end. It no longer ends before the first finish is forwarded. It still ends before `close(tee)`, so a consumer that reads to EOF observes a recorded generation.

The case #438 fixed (a consumer that stops at finish and cancels) stays a success, by D1's snapshot. What changes is narrower: a consumer that closes or shuts down the shared client without draining to EOF or waiting for `OnRecordComplete` can now lose the generation. The Gateway's `Close` already waits on `OnRecordComplete`.

A consumer that stops reading without canceling keeps the tee goroutine blocked on `tee <- part` once the 64-part buffer (`streamRecordingBuffer`) is full. That goroutine block already happens today. The recording then ends when upstream closes while the remaining parts fit in the buffer, or when the context ends. Before this change, the generation had already been enqueued at the first finish. A context that never ends therefore means no generation. This is documented in godoc and `docs/middleware/agent-observability.md`.

### D6. Gateway spec, test and docs

The Gateway currently documents the first-finish export as a limitation:
- `gateway-text-observability` requirement "Agent Observability first-segment limitation", with its scenario "Multi-segment stream exports the first segment";
- in "Logical text lifecycle semantics", the scenarios "Usage is split across stream parts" and "Stream with several response segments" limit the generation to the first finish;
- in "Logical finalization precedence and strongest usage", the scenario "Agent Observability stops at the first finish", and a precedence rule that makes cancellation or timeout the generation's call error even after a finish;
- the `ai-gateway/docs/text-observability.md` "Export to Agent Observability" sentence saying the export records only the first message or response;
- `TestModelObservationChain_SequentialSegments`, which asserts generation input 7, output 2 and `stop_reason` "tool_calls" in all three cases.

The `gateway-text-observability` delta restates D1 at the Gateway boundary:
- MODIFIED "Logical text lifecycle semantics": same scenario names, with the generation covering every segment.
- REMOVED "Agent Observability first-segment limitation".
- REMOVED "Logical finalization precedence and strongest usage", replaced by ADDED "Logical finalization precedence across response segments". OpenSpec's MODIFIED cannot drop a scenario, and "Agent Observability stops at the first finish" no longer holds. The replacement keeps the precedence, pass-through and usage rules and the scenario "Logical finalization precedence and strongest usage".
- ADDED "Agent Observability generation covers every response segment": one generation per stream, observing every segment and finalizing at stream close or at an idle timeout, deadline or cancellation. It carries the last finish's usage, the last finish reason, first-output timing from the first segment, and completion at stream end on normal close.
- ADDED "Agent Observability generation after a finish": D1's call-error rule and the last-finish snapshot, completed at that finish.

`TestModelObservationChain_SequentialSegments` then asserts:
- normal close: one generation with input 4 and output 9 (the second finish) and `stop_reason` "stop", and the same usage in the log record;
- deadline and idle stall after the first finish: input 7, output 2, "tool_calls" and no `call_error`, with the same usage in the log record, while logs and metrics keep timeout and cancelled;
- a new deadline case where a provider `PartError` (`provider.NewAPICallError`, status 503) precedes segment 1: `call_error` "server_error" (the agento11y metadata-only classification), with input 7, output 2 and "tool_calls", while metrics keep the deadline classification and logs keep the provider error.

`docs/concepts/gateway-architecture.md` (the stream lifecycle paragraph at :223-228 and its flowchart) and the `docs/concepts/gateway-glossary.md` "Response segment" entry (:52) describe the runtime ("a `finish` ends a segment, not the stream"), not the export. Neither mentions the Agent Observability first-segment export, so no edit is expected; the task re-checks them for drift.

### D7. Release and dependency scope

The released modules that change are the root module (`internal/streamusage`), `middleware/logger`, `middleware/prometheus` and `middleware/agentobservability`. release-please attributes the change to each of them by the files it touches.

- **PR title:** `feat(middleware)!: record every response segment of one stream`, with a `BREAKING CHANGE:` footer describing D2's usage rule and D5's timing contract.
  - `!` marks the removed finish-before-forward guarantee for direct consumers and the reversed "lower finish usage" rule for every stream-summary middleware.
  - Under the alpha prerelease configuration (`release-please-config.json`), this is expected to advance the prerelease counter rather than produce a stable major. Confirm with `mise run release-preview`.
- **Gateway edits:** the D6 spec, test and doc updates land in the same PR. Task 5.4 checks any Gateway plugin release attribution they cause.
- **Module versions:** never hand-edit versions or changelogs. `ai-gateway/go.mod` builds same-revision source through `replace ../middleware/agentobservability`. The release PR updates its `require` (CONTRIBUTING "Every `go.mod` replaces…"), so no manual bump is needed. Today `ai-gateway/go.mod` already requires `middleware/agentobservability v0.1.0-alpha.2`, which equals `.release-please-manifest.json`, so nothing needs bumping.
- **agento11y:** `github.com/grafana/agento11y/go` and `go-providers/anthropic` are already at v0.19.0, the latest Go tag (`go list -m -versions`, 2026-10-28). agento11y's unreleased `main` adds only tool and skill tracing changes. A task re-checks this at implementation time, and bumps both modules together in `middleware/agentobservability` and `ai-gateway` if a newer release exists.

### D8. Parity classification

The recording surface is a Grafana-only Go extension (`test/conformance/PARITY.md` "Agent Observability integration | Go extension | mixed"). Usage selection now follows upstream `ai@7.0.143` `streamText` and its OTel integration: the last finish wins (D2). That moves the stream-summary middleware closer to upstream semantics. The registered baseline `test/conformance/upstream.yaml` is unchanged, and so are the fixtures. This is a parity-preserving Go extension change.

An upstream-fixture replay is not added:
- this module's conformance harness (`conformance_test.go`, `testdata/stream/*`) maps its own stream inputs;
- the upstream provider-parts decoder lives in the `ai-gateway/providerwire/v4` test package (`stream_segments_replay_test.go`), which this module cannot reach.

The fixture numbers serve as design evidence only. Focused synthetic unit tests cover the behavior.

## Risks / Trade-offs

- [A direct consumer closes the shared client right after reading `finish`, without draining or waiting] → Release notes (`!`/BREAKING footer), godoc and docs describe draining or `OnRecordComplete`. The adapted `TestRecordingMiddleware_StreamStopAtFinishThenCancelRecordsSuccess` (task 2.2) pins the supported pattern.
- [Multi-segment usage is understated] → It matches upstream `streamText`, and logs, metrics and the generation stay consistent. Docs state that usage is the last message's, not a sum.
- [A finish reports a lower counter than an earlier part of its response] → The finish now wins, as in upstream. No in-repo provider does this; a finish that omits a counter keeps the segment's greatest value.
- [A snapshot at every finish adds work on the stream goroutine] → It copies only stream-derived state, so prompt conversion and media encoding run once, at finalize. The cost is O(output) per finish for the built output, which is negligible for realistic segment counts (15 in the largest fixture).
- [Snapshot capture panics] → It runs under `streamrecover.Recover`. The snapshot is marked invalid, and `finalize` records the current state with `streamRec.CallError()`, never `ctx.Err()` after a finish. The stream passes through unchanged. The guard is defensive: it has no reachable trigger after D4, and no test covers it; no test seam is added.
- [A custom `GenerationFilter` drops the snapshot `CompletedAt`] → agento11y falls back to `End` time, which is documented.
- [Logs and metrics report timeout or cancellation after a finish, while the generation reports success] → The Gateway spec states this explicitly ("Agent Observability generation after a finish" and "Logical finalization precedence across response segments"). The generation describes completed model output, and logs and metrics describe the request outcome. All three report the same usage.

## Migration Plan

No data or configuration migration is needed. Rollback means reverting the PR, which restores first-finish finalization.
