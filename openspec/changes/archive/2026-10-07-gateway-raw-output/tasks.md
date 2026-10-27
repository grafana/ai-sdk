## 1. Runtime

- [x] 1.1 Pass `includeRawChunks` to the selected model and remove the raw-output capability error.
- [x] 1.2 Write requested raw parts as `raw` events under existing frame and part limits; drop unrequested and value-less parts.
- [x] 1.3 Project MCP credential echoes out of raw values.
- [x] 1.4 Add the `raw` event to the stream-event schema and remove the error enum entry.

## 2. Evidence

- [x] 2.1 Unit tests for credential projection and value preservation.
- [x] 2.2 Handler tests for flag propagation, filtering, ordering, value-less parts and oversized events.
- [x] 2.3 Real-handler tests with the registered TypeScript client and the Go client in both call modes.
- [x] 2.4 Telemetry test showing raw values stay out of logs, metrics and Agent Observability exports.

## 3. Documentation

- [x] 3.1 Describe raw output in the Grafana Gateway provider guide.
