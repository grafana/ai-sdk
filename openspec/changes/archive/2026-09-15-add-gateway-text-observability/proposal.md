## Why

The authenticated text Gateway can currently expose HTTP health telemetry, but model calls still bypass the reusable logging, Prometheus, and Agent Observability middleware and therefore cannot be operated as one canonical public generation. Work package 8 must add that logical telemetry before fallback is introduced, because work package 9 needs a stable boundary between one public call and its private physical attempts.

## What Changes

- Add a Gateway-owned request-observation context populated only from verified authentication, a Gateway-generated correlation ID, and explicitly trusted deployment configuration.
- Wrap each startup-constructed public model exactly once with the fixed logical chain: approved context enrichment, Agent Observability recording, structured logging and Prometheus, canonical public identity, then the direct provider.
- Configure model logs, model metrics, and a bounded Agent Observability exporter from explicit service settings, with metadata-only content capture and fail-open telemetry delivery.
- Force canonical public provider/model identity for all logical telemetry and omit credentials, request/response payloads, backend identity, topology, and provider-originated error text.
- Record unary and streaming text lifecycle, outcome, usage, duration, and time to first output without changing ProviderWire request/response bytes or its stream ownership rules.
- Add reusable requested-identity controls to structured logging and Agent Observability middleware; their existing response-preferred defaults remain unchanged for current SDK consumers.
- Reserve physical candidate attribution and retry decisions for work package 9, per-request Agent Observability controls for work package 27, and later content/event families for their owning capability packages.

## Capabilities

### New Capabilities

- `gateway-text-observability`: Gateway composition, verified request metadata, exporter configuration, canonical logical text telemetry, privacy, lifecycle, and work-package seams.

### Modified Capabilities

- `structured-logging-middleware`: Add requested-identity and bounded stream-cleanup controls needed by a logical routed service.
- `prometheus-middleware`: Add bounded stream-cleanup control for service composition; its existing requested-identity mode is reused unchanged.
- `agent-observability-middleware`: Add requested-identity and bounded stream-cleanup controls needed by a logical routed service.

## Impact

- AGPL Gateway service configuration, authenticated request context, catalog construction, process lifecycle, telemetry registry, and real-command tests under `ai-gateway/`.
- Apache nested middleware modules `middleware/logger` and `middleware/agentobservability`, with additive options and unchanged defaults.
- New pinned module dependencies from `ai-gateway` to the released or immutable-pinned logger, Prometheus, and Agent Observability middleware modules; the root SDK remains independent and `ai-gateway` stays outside `go.work`.
- No ProviderWire V4 schema, wire behavior, public error, Go Gateway client, image/capacity, fallback, deployment, or later-capability implementation is included.
