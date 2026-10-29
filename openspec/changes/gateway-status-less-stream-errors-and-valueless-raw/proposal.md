# Proposal

## Why

Direct providers emit two native stream shapes that ProviderWire V4 rewrites today. One is an in-stream provider error with no HTTP status, which becomes a retryable `502 upstream failure`. The other is a raw part without a value, which is dropped. When a provider stream ends right after an error part without a `finish`, the Gateway also adds a synthetic `500 internal error`. Comparing Gateway output with the registered upstream goldens shows all three differences (#564), for both registered clients, on `anthropic/upstream/spliced-message-start` and `openai/upstream/parallel-tool-call-wrapper`.

## What Changes

- **Status-less stream error.** An in-stream provider error whose `APICallError` has status 0 and no cancellation, deadline or transport cause is written as a new fixed safe error document. The document has no `statusCode`, the fixed message `provider stream error`, type `internal_server_error`, code `provider_error`, and `retryable: false`. It replaces today's implicit `upstream` fallback for that case only. Status-bearing, malformed-status, cancellation, timeout and transport classification do not change. Unary and stream-setup classification do not change.
- **Value-less raw part.** When the caller requested raw output, a `PartRaw` with no value is written as `{"type":"raw"}`. Values that are invalid JSON, invalid UTF-8, or repeat a key are still dropped.
- **Clean EOF after a provider error.** When the provider closes its stream and the last frame written was a provider error frame, the response ends at clean EOF. Any other premature close keeps its single synthetic internal error.
- **Go client.** `providers/grafana` accepts the status-less document only for its registered type and code. It yields a `PartError` whose `APICallError` has status 0 and is not retryable. It also accepts `{"type":"raw"}` and yields a `PartRaw` with a nil value when raw output was requested. No public Go API is added.
- **Schema and evidence.** The production stream-event schema accepts both shapes. Go handler and client tests and the pinned-client contract tests cover them. Both clients are shown to receive no native error text in base error fields.
- **Superseded rationale.** The fixed-document rule stays, extended by one document. The drop rule for value-less raw parts is removed. Premature EOF stays a terminal internal error except right after a provider error frame.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `providerwire-v4-streaming-runtime`: changes in-stream provider errors (adds the status-less base shape and classification) and premature close (clean EOF right after a provider error frame).
- `gateway-execution-runtime`: the closed failure classification gains a stream-only provider-stream category for status-less in-band errors.
- `gateway-raw-output`: a requested raw part without a value is written as `{"type":"raw"}` instead of being dropped.
- `grafana-gateway-client`: decodes the status-less stream error and the value-less raw part.

## Impact

- **AGPL `ai-gateway`:**
  - `internal/execution` classification;
  - `providerwire/v4` (errors, error documents, stream state machine, `schema/stream_event.json`);
  - the contract testserver and TypeScript contract tests under `test/providerwire-v4`;
  - the test-only adapter's documents;
  - the `cmd/grafana-ai-gateway/internal/service` fallback-acceptance and generated-file tests, whose error-then-close rows lose their trailing `internal_error`.
- **Apache `providers/grafana`:** stream error and raw decoding. There is no exported API change.
- **Docs:** the Grafana Gateway provider guide and the Gateway architecture walkthrough.
- **Unchanged:**
  - the registered upstream baseline (`ai` 7.0.143, `@ai-sdk/gateway` 4.0.116, `@ai-sdk/provider` 4.0.29, `@ai-sdk/openai` 4.0.96, `@ai-sdk/anthropic` 4.0.82);
  - provider recordings, `knownGaps` and `PARITY.md`;
  - the TypeScript client, because `@ai-sdk/gateway` enqueues decoded parts unchanged.
- **Compatibility:** already released `providers/grafana` clients treat both new shapes as terminal protocol errors. They expose nothing, but on streams that continue after a status-less error they lose the later content and finish. Rollout releases the client before the Gateway; see design.md, Risks and "Confirmed decisions".
- **Consumers:** root `fallback` with `WithBufferedStream` no longer falls back on these errors, because they are not retryable, and `middleware/prometheus` reports `status_code` `none` instead of `502`.
- **Evidence:** all acceptance evidence lives on `main` (Go unit tests, stream-event schema tests and the pinned-client contract suite). No external conformance harness is required.
