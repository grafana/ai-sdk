## Why

Anthropic and OpenAI SDK-backed adapters discard caller-visible transport metadata and requested raw stream events; Anthropic also drops per-call headers, including the Agent marker. Issue [#229](https://github.com/grafana/ai-sdk/issues/229) needs returned-result evidence beyond conformance request capture, without replacing SDK authentication or leaking native transport details through Gateway.

## What Changes

- Forward Anthropic call headers in unary and streaming requests, with ordinary call-over-constructor precedence and a normalized union of configured, call, and feature-required beta headers. Preserve existing OpenAI call-header behavior.
- Populate existing provider request/response fields with the final SDK outbound JSON body, actual HTTP response headers, and full unary response JSON. Make streaming metadata available through core step/result surfaces.
- Implement opt-in frame-level raw events in both adapters, including Anthropic pings, valid JSON decoding failures, and post-preflight provider error frames, preserving raw-before-normalized ordering and initial error detection.
- Represent syntactically invalid JSON with an absent raw value followed by an error, never invalid bytes in `json.RawMessage`.
- Preserve SDK clients, authentication, retries, configured transports, cancellation, and decoder ownership, including Vertex and preconfigured OpenAI/Mantle integration.
- Add deterministic HTTP/result and lifecycle regression tests; leave Gateway privacy and raw-output policy unchanged.

## Capabilities

### New Capabilities

- `sdk-provider-transport-context`: Shared native Anthropic/OpenAI contract for call headers, returned transport metadata, raw event selection/ordering, transport ownership, and core visibility.

### Modified Capabilities

None. Existing provider, core lifecycle, and Gateway contracts remain in force; the new capability adds native transport-observability requirements.

## Impact

- Affected implementation: `providers/anthropic/model.go` and response/stream conversion, `providers/openai/model.go` and stream pump/preflight/conversion, and focused provider/core integration tests.
- Uses existing `provider.CallOptions`, `RequestMetadata`, `GenerateResponse`, `ResponseHeaders`, and `PartRaw`; no new public API or SDK dependency upgrade is proposed.
- Reference: registered upstream commit `4e8c387622ee1bb0d55841664416d38754d5c9a3`, Anthropic 4.0.59 / OpenAI 4.0.72. Reporting final outbound JSON is an explicit observability adaptation where upstream reports logical/pre-transform request arguments.
- Non-goals: upstream pin upgrades, request/provider-options redesign, model capabilities, full TypeScript schema-validation parity, Gateway raw-output enablement (#116), or ProviderWire redesign.
