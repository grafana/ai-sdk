## Why

Issue [#229](https://github.com/grafana/ai-sdk/issues/229) is still valid: at the registered upstream baseline (`@ai-sdk/anthropic` 4.0.58, `@ai-sdk/openai` 4.0.71, commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`), both providers return request/response transport metadata and opt-in raw stream events, and Anthropic applies per-call headers. The Go SDK-backed adapters still drop successful transport metadata and raw events; Anthropic also drops per-call headers, including Agent's marker. Replay-server request snapshots do not test the returned fields.

## What Changes

- Preserve `CallOptions.Headers` on direct and Vertex Anthropic unary/streaming requests: ordinary same-name headers use case-insensitive call override, while `anthropic-beta` unions/deduplicates configured and per-call tokens as in the pinned upstream; retain the official SDK's client, transport, and authentication paths.
- Populate existing provider result fields with outbound request JSON and actual HTTP response headers for Anthropic and OpenAI, in generate and stream modes. Preserve unary response JSON body where the existing contract supports it; do not add a body to streaming responses.
- Emit `PartRaw` only when `IncludeRawChunks` is true, before normalized or error parts for observable decoded events; an observable syntax-invalid event has an absent raw value. Distinguish SDK-hidden Anthropic frames as an unproven parity boundary, retain preflight failure, and make cancellation release the stream even when the consumer stalls.
- Make `StreamText` expose streaming result headers on step/result response surfaces without exposing provider-private metadata through the Gateway or changing frontend SSE wire shape.
- Test returned metadata, header precedence and raw ordering with deterministic fake HTTP transport and core result tests; use only authentic inputs in provider-recorded conformance fixtures.

## Capabilities

### New Capabilities

- `sdk-provider-transport-context`: Success-path transport metadata, Anthropic per-call headers, and opt-in raw events for the official-SDK Anthropic/Vertex and OpenAI Responses adapters.

### Modified Capabilities

- `stream-text-lifecycle`: Step/result response headers must reflect a provider's streaming `StreamResult.Response` when no event-level headers supersede them.

## Impact

`providers/anthropic/model.go`, `convert_response.go`, `convert_stream.go`; `providers/openai/model.go`, `convert_response.go`, `convert_stream.go`, `stream_preflight.go`; `streamtext.go` and focused provider/core tests. Existing `provider.CallOptions`, `GenerateResult`, `StreamResult`, `StreamPart`, and `StepResult` types suffice; no new public API, SDK replacement, baseline upgrade, or Gateway normalization change. This does not implement #216, #220, #31, #116, or ProviderWire redesign #80/#69. Gateway privacy regression tests remain required.
