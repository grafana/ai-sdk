## 1. Upgrade and mapping

- [x] 1.1 Upgrade the Agent Observability module to agento11y v0.18 and update nested dependencies.
- [x] 1.2 Mark reported input-token totals as inclusive and regenerate generation snapshots.
- [x] 1.3 Normalize Bedrock and Anthropic Vertex names at the Agent Observability and OTel boundaries.

## 2. Hook compatibility

- [x] 2.1 Encode hook test responses with the v0.18 server wire shape.
- [x] 2.2 Treat an empty server transform as no transform.
- [x] 2.3 Reject changed or new transformed user messages that could hide unsupported v0.18 response roles.

## 3. OTel coverage

- [x] 3.1 Cover unary generation content, controls, identity, usage, context, and parentage.
- [x] 3.2 Cover streaming context, lifetime, pass-through, errors, metadata-only capture, and cancellation.
- [x] 3.3 Prove client shutdown invokes the configured flusher before tracer-provider shutdown.

## 4. Contracts and documentation

- [x] 4.1 Document application-owned exporters, sampling, flushing, shutdown, and delivery limits.
- [x] 4.2 Update the Agent Observability middleware specification and archive its change artifacts.
- [x] 4.3 Register `@ai-sdk/otel@1.0.65` and classify field-level parity differences.

## 5. Validation

- [x] 5.1 Run middleware, conformance, parity, vet, lint, documentation, formatting, and full test checks.
- [x] 5.2 Review the final diff and remove unrelated workspace changes.
