# Proposal

## Why

One provider `DoStream` result can carry several native responses or messages in sequence (#393). Seven imported upstream fixtures do this:

- five Anthropic fixtures with one `response-metadata` and one `finish` per message;
- two OpenAI fixtures with one `response-metadata` per response and a single `finish` at EOF.

The Gateway stream runtime accepts at most one early `response-metadata`, ends the response at the first `finish` and suppresses everything after it. Against the unchanged direct goldens this fails 14 client rows, seven fixtures for each of the two registered clients, identically for both:

- the five Anthropic rows are silently truncated at the first `finish`;
- the two OpenAI rows end with `internal_error` at the second `response-metadata`.

Both clients reproduce the unchanged direct goldens when they are served the complete part sequence. #397 has since committed the upstream-generated provider parts for all seven fixtures (`expected-provider-parts.jsonl`), so the Gateway's input contract for these cases is fixed. What remains is the Gateway lifecycle contract itself.

## What Changes

- **BREAKING (stream lifecycle): the stream ends when the provider stream ends, not at the first `finish`.** A response ends at one of:
  - provider channel close;
  - a terminal adapter or runtime failure;
  - idle timeout, request deadline or cancellation;
  - writer failure.

  A provider close is a clean EOF only when the last lifecycle part is a valid `finish`. Raw parts and provider error parts are not lifecycle parts. A close with no `finish`, or with a segment opened after the last `finish`, stays the single synthetic premature-close `internal_error`.
- **Response segments.**
  - The public start opens the first segment.
  - Every accepted `response-metadata` opens a new segment. It is accepted only when nothing is open: no text, reasoning or tool-input block, and no unfinished tool preview series. This is the same condition a `finish` requires.
  - `response-metadata` while anything is open fails safely. This includes malformed overlapping message starts.
  - Repeated `response-metadata` and `finish` parts are forwarded in place.
- **After a `finish`.**
  - Only `response-metadata`, raw parts (under gateway-raw-output), provider error parts or provider close may follow.
  - A provider error part after a `finish` is forwarded in place and does not change lifecycle state, as everywhere else.
  - Any other part, including a second `finish`, fails safely with one terminal `internal_error` after the written `finish`.
- **Timeouts and cancellation after a `finish`.** Idle timeout and request-deadline expiry keep the normal timeout error, which follows the `finish` when one was already written; request cancellation keeps today's behavior, with a cancellation error only while the writer is usable. The written `finish` is never retracted. Only provider close depends on whether the last lifecycle part is a valid `finish`, so a stall between segments is never reported as a complete response.
- **Identifier scope.**
  - Text and reasoning IDs are unique within a segment and may repeat in a later one. Anthropic restarts block IDs per message.
  - Tool-call IDs and approval IDs stay unique for the whole stream, so a provider-executed call can receive its result in a later segment.
- **Usage and execution metadata per `finish`.**
  - Each `finish` forwards its own usage, `usage.raw` and `providerMetadata` unchanged, including Anthropic `safeguardResults`. Nothing is summed or merged on the wire.
  - Every written `finish` carries the optional `providerMetadata.gateway.execution` overview, without buffering or read-ahead.
- **Bounds and ownership unchanged and stream-global.**
  - The request deadline, the idle timer, cancellation, the bounded drain and the single cleanup owner apply across all segments.
  - No part, segment, byte or frame limit is reintroduced.
  - Later segments never re-enter candidate selection, fallback or replay.
- **Observability.**
  - Logs and metrics already observe every part, so they see all segments.
  - Agent Observability finalizes its generation at the first `finish`, in the separately versioned `middleware/agentobservability` module. Until #582 fixes that, its generation reflects only the first segment. The gateway-text-observability requirement states this precisely, and the module fix is registered as its own work package.
- **Evidence.**
  - Focused native-part, raw-wire and schema tests.
  - A handler replay of the committed `expected-provider-parts.jsonl` for the seven fixtures.
  - Both clients replaying the seven fixtures through the real adapters against their unchanged `expected.jsonl` and `expected-requests.jsonl`.
- **Docs.** The stream flowchart in `docs/concepts/gateway-architecture.md`, a glossary row for response segments, and in `docs/providers/grafana-gateway.md` the execution-overview paragraph plus a note that one stream may carry several `response-metadata` and `finish` events and that EOF follows provider close.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providerwire-v4-streaming-runtime`:
  - adds the response-segment lifecycle: segment boundaries, per-segment identifiers, parts allowed after a `finish`, completion on provider close, timeouts and cancellation after a `finish`, per-`finish` usage and enrichment, and stream-global bounds;
  - adds the sequential evidence requirement;
  - removes immediate EOF and post-finish suppression;
  - rewrites the metadata, text-ID, finish-authority, provider-error, terminal-error, cancellation and race requirements that assumed one response.
- `gateway-streaming-function-tools`: `response-metadata` follows the segment rule instead of preceding all content, tool IDs stay stream-global, and the stream no longer ends immediately at `finish`.
- `gateway-provider-metadata`: a written `finish` keeps its metadata and position, and later parts follow the segment rules instead of being suppressed.
- `gateway-sources`: a source no longer forbids later `response-metadata`, and a source after a `finish` without new metadata fails safely instead of being suppressed.
- `gateway-custom-content`: a custom event no longer forbids later `response-metadata`.
- `gateway-text-observability`: states how logs, metrics and Agent Observability observe a stream with several segments.

## Impact

- **Code:**
  - `ai-gateway/providerwire/v4`: `stream.go` (`streamState`, `processStreamPart`, `consumeStreamParts`), `reasoning.go` and `stream_tools.go`, plus their tests;
  - the ProviderWire testserver (`ai-gateway/test/providerwire-v4/testserver`) and `runtime-integration.test.ts`;
  - the test-only Go capture tool in `providers/grafana/internal/capture`, so the Go client can replay fixtures that declare provider tools.
- **Not changed:**
  - the root SDK, the other provider modules, the `providers/grafana` library and middleware modules;
  - provider inputs, goldens, `upstream.yaml` `knownGaps` and `PARITY.md`.
- **Wire:**
  - The event types and their schemas are unchanged.
  - Clients now receive every segment, repeated `response-metadata` and `finish` events, and EOF at provider close instead of at the first `finish`.
  - A part after a `finish` that is not allowed now yields a terminal `internal_error` instead of silent suppression.
  - The pinned `@ai-sdk/gateway@4.0.116` client and the Go Grafana client already forward every part and do not close on `finish`.
- **Latency:** EOF follows provider close. A provider that never closes after its last `finish` holds the response until the idle timeout or the request deadline and then ends with a timeout error, where it previously ended cleanly at once. The `finish` itself is still written immediately.
- **Release:** the PR title is `feat(ai-gateway)!: …` because the public stream lifecycle changes. The Agent Observability follow-up carries its own module scope.
- **Delivery:** this lands as its own PR after #397 (design.md D11).
- **Out of scope:**
  - the Anthropic adapter's overlapping-start handling;
  - OpenAI compaction `custom` and MCP `tool-approval-request` coverage;
  - changing the strongest-value usage semantics of logs and metrics;
  - live-provider acceptance.
