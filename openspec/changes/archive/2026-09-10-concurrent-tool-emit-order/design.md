## Context

`openspec/changes/archive/2026-04-07-concurrent-tool-execution/` introduced concurrent tool execution. Its Decision 3, "Fire-as-complete event ordering", chose to emit `StreamToolResult` and `StreamToolError` from within each goroutine as tools complete, and rejected "collect all results, then emit in original call order" by name, citing latency and divergence from upstream. The merged requirement at `openspec/specs/concurrent-tool-execution/spec.md:70` says the same thing.

The implementation does the opposite. `executeTools` waits on every goroutine, then walks the outcomes once, emitting each event and appending to `step.ToolResults` in the same pass (`streamtext.go:1650-1659`, and again on the approval-resume path at `:2001-2005`). One loop is doing two jobs, and `spec.md:84` requires the second of them in call order, so satisfying `:84` by waiting is what breaks `:70`.

This change is therefore not a reversal of Decision 3. It is Decision 3 finally being implemented. The comment on `toolExecOutcome` at `streamtext.go:1451-1453` justified the batching as matching upstream; upstream does the opposite, enqueuing inside each per-tool `async` callback (`execute-tools-from-stream.ts:229` in `ai@7.0.65` at `d76eb85a`) and restoring call order later, only for the provider messages, via `sortToolResultContentByToolCallOrder` (`to-response-messages.ts:222-257`).

## Goals / Non-Goals

**Goals:**
- Emit each tool's stream event as that tool completes, satisfying `spec.md:70`
- Keep `step.ToolResults` and the resume path's `approvedToolParts` in tool call order, satisfying `spec.md:84`
- Keep the conformance suite green and deterministic, without weakening it where determinism is real
- Preserve the existing `OnChunk` contract, which upstream serializes

**Non-Goals:**
- Changing `step.ToolResults` ordering. Upstream's own `step.toolResults` is in completion order (`step-result.ts:401`), so Go's `spec.md:84` is an unrecorded deviation reaching the same provider wire by a different route. Out of scope here, worth its own issue.
- Giving `r.emit` a cancellation path. It is a bare channel send, so an abandoned `FullStream()` already stranded a goroutine forever; this change raises that from one goroutine to one per tool. The fix is a `select` on `ctx.Done()`, which would also make the escape hatch documented at `docs/guides/streaming-http.md:100` true for the first time. Filed separately rather than smuggled in here.
- Per-fixture opt-in for comparator relaxation. Considered and rejected below.

## Decisions

### Decision 1: Emit from a deferred call in `executeSingleTool`

**Choice**: A single `defer` at the top of `executeSingleTool` emits `outcomes[index].event` if one was recorded.

**Alternatives considered**:
- Emit at each of the three points where an outcome is written. Triplicates the call and invites a future fourth write path that forgets it.
- Emit from the caller's goroutine after each `wg.Done()`. Reintroduces the coupling this change exists to remove.

**Rationale**: One emit site, covering every return path including error returns. The closure registers `defer wg.Done()` before calling `executeSingleTool`, so the emit defer always runs first and tool outputs can never cross `finish-step`.

### Decision 2: Serialize the emit and the `OnChunk` callback under `emitMu`

**Choice**: A dedicated mutex holds `r.emit` and `r.callOnChunk` together, in a `publish` helper used by every site that sends a part and then invokes `OnChunk`.

**Alternatives considered**:
- Leave both unsynchronized, as archived Decision 5 does for `OnToolCallStart` and `OnToolCallFinish`. This was the initial implementation and it was wrong: a user `OnChunk` callback became concurrent, the repo's own idiom of an unsynchronized counter (`streamtext_test.go:2471`) raced under `-race`, and the `OnChunk` sequence stopped matching the `FullStream()` sequence in roughly a quarter of runs.
- Reuse the existing `r.mu`. That lock guards the step accumulators and is held across other work, so taking it around a blocking channel send invites lock-order problems.

**Rationale**: Decision 5 covers the tool-lifecycle callbacks, which upstream also fires from inside concurrent promises. `OnChunk` is a different contract: upstream invokes it inside the stream transform (`stream-text.ts:1196`), serially and in stream order, and documents that processing pauses until it resolves. Nothing in `spec.md` or `options.go:404` states a threading contract for `OnChunk`, so the safe reading is the upstream one. Decision 5's stated reason for not synchronizing, that it "would serialize execution and negate the concurrency benefit", does not apply: `emitMu` serializes only the publish step, not tool execution. Measured, four tools sleeping 500ms, 20ms, 20ms, 20ms still deliver the first result at 22ms against 502ms when batched.

### Decision 3: Relax the chunk comparator for locally-executed tool outputs only

**Choice**: `CompareChunks` sorts each maximal run of adjacent `tool-output-available` and `tool-output-error` chunks by `toolCallId`, excluding any chunk carrying `providerExecuted: true`, which also ends a run.

**Alternatives considered**:
- Keep the comparator strict. Not viable: the recorded fixtures' tools resolve instantly, so with concurrent emission the arrival order is scheduler-dependent. That buys a flaky required check, not a stably red one, and regenerating the fixtures does not help because they are generated from upstream `streamText` with instant tools and the fixture config schema has no delay field.
- Key only on chunk type. This was the initial implementation and it was wrong. It also relaxed two provider-executed fixtures whose order comes off the recorded wire and is deterministic. In `anthropic/upstream/web-fetch-tool-20260209` that order is load-bearing, an inner `web_fetch` result embedded in an outer `code_execution` result, and the `toolCallId`s sort the other way, so the comparator actively rewrote the recorded order. Swapping those two chunks failed before the change and passed after it.
- Per-fixture opt-in in `config.yaml`. Narrower still, and defensible. Rejected because the property that licenses relaxation is a property of the chunk, not of the fixture: a locally-executed tool output is emitted from a goroutine wherever it appears. A fixture flag would have to be set correctly on every future fixture, and forgetting it produces a flake rather than a failure.

**Rationale**: `runner.go` already carries two order-insensitive normalizers, `normalizeJSONValue` for tool declaration arrays and `normalizeBetaHeader`, each backed by a requirement in `conformance-testing/spec.md` at `:255`, `:265` and `:288`. This is the fourth in that family and follows the same shape. `tool-output-error` is included because it leaves the same goroutine, and a step mixing one success with one failure would otherwise break the run and normalize nothing.

## Risks / Trade-offs

- **[Weakened ordering check between sibling tool outputs]** -> The relative order of adjacent locally-executed tool outputs within one step is no longer compared. -> Mitigation: everything else stays positional. Verified by mutating a fixture five ways: swapping two output values with `toolCallId`s in place, moving an output across `finish-step`, deleting an output, reordering two `tool-input-start` chunks, and swapping two provider-executed outputs. All five still fail; only the intended sibling swap passes.

- **[`OnChunk` now runs under a lock]** -> A slow user callback blocks the tool goroutine that is publishing, and therefore its siblings' publishes. -> Mitigation: upstream has the same property by construction, since it awaits `onChunk` inside the transform. Tool execution itself is unaffected.

- **[Emit blocking is amplified]** -> `r.emit` has no cancellation path, so a consumer that abandons `FullStream()` now strands one goroutine per tool instead of one in total, and `wg.Wait()` never returns. -> Mitigation: none in this change. The contract at `docs/best-practices/production.md:43` already requires draining. Filed separately.

- **[`providerExecuted` is a proxy, not a precise marker]** -> The comparator treats "not provider-executed" as "emitted from a tool goroutine". `rejectToolCall` breaks that: it publishes `tool-output-error` from the run goroutine, deterministically, without the flag. -> Mitigation: none needed today. A run only forms between adjacent chunks, and no fixture places a rejection error next to a tool goroutine's output; the scan finds exactly three relaxed runs, all genuine concurrent sibling pairs. Keying on this exactly would need a wire-level marker for "emitted concurrently", which does not exist.

- **[Go is nondeterministic where upstream is deterministic]** -> Upstream with instant tools resolves ties in call order because it is single-threaded with uniform microtask depth. Go's scheduler does not. This is a category 2 intentional deviation and is recorded in `test/conformance/upstream.yaml` and `test/conformance/PARITY.md`.
