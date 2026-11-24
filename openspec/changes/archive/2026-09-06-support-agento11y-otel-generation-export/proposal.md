## Why

The Agent Observability middleware already pins agento11y v0.19.0, which can export generations as OpenTelemetry generation spans. The middleware has no tests or setup documentation for that protocol. The v0.19.0 hook decoder also changes response data before middleware validation, which the middleware must handle explicitly.

## What Changes

- Export unary and streaming generations through the protocol configured on the resolved client, including OpenTelemetry generation spans.
- Record the mapped request when the provider call fails before it returns a result or stream.
- Normalize the AI SDK Bedrock and Anthropic Vertex provider identifiers before Agent Observability export and in hook requests. This changes stored provider values for every export protocol.
- Align hook tests with the v0.19.0 server response wire. The dependency normalizes empty HTTP `transformed_input:{}` to nil, so allow remains pass-through.
- **BREAKING**: Temporarily reject every non-nil decoded `TransformedInput` with `ErrHookTransformFailed` before provider invocation, regardless of `FailOpen`. This includes valid assistant, system-prompt, tool-message, tool-list, and user changes, and unchanged replacements.
- Keep allow without a transform and deny behavior unchanged. Preserve reconstruction rules as inactive constraints, not active transform support.
- Require a decoder that preserves invalid data for validation or rejects it before re-enabling transforms. V0.19.0 changes unsupported roles and drops malformed parts before middleware validation; no raw response API is available.
- Document application ownership of exporters, sampling, flushing, shutdown, and remote delivery.
- Record field-level differences from `@ai-sdk/otel@1.0.153`, the release that depends on the registered `ai@7.0.153`. `@ai-sdk/otel` stays unregistered in `test/conformance/upstream.yaml`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-observability-middleware`: Add protocol-selected generation spans, provider normalization, v0.19.0 hook decoding constraints, and application-owned OpenTelemetry lifecycle requirements.

## Impact

The change affects `middleware/agentobservability`, its tests and hook fixtures, the Agent Observability guide, and the capability specification. It does not change go.mod, `test/conformance/upstream.yaml`, provider requests, provider stream parts, UI chunks, Server-Sent Event framing, or frontend behavior.
