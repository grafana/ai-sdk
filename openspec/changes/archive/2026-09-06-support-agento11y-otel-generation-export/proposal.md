## Why

The Agent Observability middleware uses agento11y v0.15, so it cannot export generations through the v0.18 OpenTelemetry path. The upgrade also changes token semantics and hook response decoding, which the middleware must handle explicitly.

## What Changes

- Upgrade the optional middleware module to agento11y v0.18.
- Export unary and streaming generations through the protocol configured on the resolved client, including OpenTelemetry generation spans.
- Mark reported input-token totals as inclusive.
- Normalize the AI SDK Bedrock and Anthropic Vertex provider identifiers before Agent Observability and OpenTelemetry export.
- Align hook tests with the v0.18 server response wire and treat an empty server transform as no transform.
- **BREAKING**: Reject changed or new transformed user messages until agento11y preserves unsupported response roles. This prevents v0.18 from silently demoting `system` or unknown roles to `user`.
- Document application ownership of exporters, sampling, flushing, shutdown, and remote delivery.
- Register the matching `@ai-sdk/otel@1.0.65` baseline and record field-level differences.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-observability-middleware`: Add protocol-selected generation spans, inclusive usage semantics, provider normalization, v0.18 hook decoding constraints, and application-owned OpenTelemetry lifecycle requirements.

## Impact

The change affects `middleware/agentobservability`, its tests and generation snapshots, the Agent Observability guide, and parity metadata. It does not change provider requests, provider stream parts, UI chunks, Server-Sent Event framing, or frontend behavior.
