# Proposal

## Why

A provider `DoStream` result can carry several native responses or messages, each ending in its own `finish`. For example, Anthropic emits one `finish` per `message_stop`, and the upstream `programmatic-tool-calling` fixture has 15 of them. The Gateway forwards all of them in one stream (#393, open PR #583). `middleware/agentobservability` ends its single generation at the first `PartFinish` and stops observing after it (`recording.go` `runStreamTee`, the `finished` flag). As a result, the exported generation's finish reason, timing, output and usage cover only the first segment ([#582](https://github.com/grafana/ai-sdk/issues/582)).

## What Changes

- The recording middleware keeps observing after every `PartFinish`.
  - The generation ends once: when upstream closes, or when the request context ends.
  - A multi-segment stream yields one generation with:
    - the last finish reason;
    - first-token time from the first segment;
    - completion time at stream end on normal close;
    - every segment's output in order.
- **BREAKING (timing contract):** a finished stream is no longer enqueued before its first `finish` part is forwarded. It is enqueued before downstream EOF.
  - If the context ends after at least one `finish` was observed, cancellation or timeout is never the call error. The middleware records the state at the last `finish`, with that finish's completion time, unless a `PartError` followed it. A provider `PartError` observed at any point stays the call error. A consumer that stops at `finish` and cancels therefore still records a successful generation when the provider reported no error.
  - Consumers that close the shared agento11y client right after reading a `finish` must first drain the stream to EOF or wait for `RecordingOptions.OnRecordComplete`.
  - Cancellation before any `finish` is unchanged.
- Usage keeps the existing strongest-value-per-counter aggregation (`internal/streamusage`) and now applies it across every segment. On normal close, logs, metrics and the generation keep reporting the same totals.
  - Summing per segment is rejected. Anthropic finish usage carries earlier messages' output counts (upstream fixture: 10347 summed vs 922 true).
  - Multi-segment usage can therefore be understated, for example input 4551 vs a true 7920 on that fixture. Exact per-message usage becomes a follow-up issue.
- Text and reasoning accumulators restart at each `finish`. An ID reused by the next segment (Anthropic restarts block IDs at `"0"`) then appends new output instead of merging into the previous segment's part.
- After #583 merges, the Gateway updates its interim first-segment limitation:
  - remove it from the `gateway-text-observability` spec and `ai-gateway/docs/text-observability.md`;
  - update `TestModelObservationChain_SequentialSegments` to the new behavior.
- No agento11y dependency bump. `github.com/grafana/agento11y/go` and `go-providers/anthropic` are already at v0.19.0, the latest Go release as of 2026-10-28. Implementation re-checks this.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-observability-middleware`:
  - stream recording spans every response segment;
  - strongest-value usage across multi-finish streams, and per-segment text/reasoning accumulation;
  - recorder end-timing moves from the first finish to upstream close or context end, with a last-finish success snapshot;
  - cancellation precedence applies only before the first finish.
- `gateway-text-observability`:
  - a multi-segment stream produces one generation with the strongest usage, the last finish reason and, on normal close, timing to stream end;
  - cancellation or timeout after a finish keeps its classification in logs and metrics, while the generation records the finished segments and keeps any provider stream error as its call error.

  #583's interim limitation wording is removed once it lands (see tasks).

## Impact

- Code: `middleware/agentobservability/recording.go` (`runStreamTee`) and `map_stream.go` (`StreamRecorder` text/reasoning accumulators, `buildOutput`). No exported API change.
- Not changed: `internal/streamusage`, `middleware/logger`, `middleware/prometheus`, providers, conformance fixtures.
- Tests:
  - `middleware/agentobservability` `map_stream_test.go` and `recording_test.go`, including adapting `TestRecordingMiddleware_StreamFinishIsRecordedBeforeDownstreamCancels`;
  - after #583, `ai-gateway/cmd/grafana-ai-gateway/internal/service/model_observability_test.go`.
- Docs:
  - `docs/middleware/agent-observability.md` and godoc on `RecordingMiddleware`/`runStreamTee`;
  - after #583, `ai-gateway/docs/text-observability.md`;
  - check `docs/concepts/gateway-architecture.md` and `docs/concepts/gateway-glossary.md` for drift.
- Release: `middleware/agentobservability` only, via release-please from the PR title. Gateway test, doc and spec edits ride along. `ai-gateway/go.mod` builds against same-revision source through its `replace`. The release PR updates its `require`.
- Parity: a Grafana-only Go extension (`test/conformance/PARITY.md` "Agent Observability integration | Go extension"). No change to the registered baseline or fixtures.
- Sequencing: depends on #583 merging first for the Gateway part.
