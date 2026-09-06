## Context

`RecordingMiddleware` already delegates generation creation to the resolved agento11y client and passes the returned context to the provider. Agento11y v0.18 adds an OpenTelemetry generation protocol that uses this existing path, but the middleware module still pins v0.15.

The upgrade also exposes two compatibility constraints. Input totals now need an explicit inclusive marker. The v0.18 hook decoder accepts numeric protobuf roles, but it converts unsupported response roles to `user` before the middleware can validate them. No patched agento11y release exists.

The registered Vercel baseline is commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, with `ai@7.0.65` and `@ai-sdk/otel@1.0.65`. Agent Observability is a Grafana-specific integration rather than a complete port of the upstream telemetry API.

## Goals / Non-Goals

**Goals:**

- Support client-selected agento11y v0.18 OpenTelemetry generation export without a middleware-owned transport API.
- Preserve provider results, stream parts, context values, and generation lifetime.
- Export standard OpenTelemetry provider names for the built-in Bedrock and Anthropic Vertex models.
- Keep malformed transformed roles from reaching providers as user messages.
- Record application ownership and field-level upstream differences.

**Non-Goals:**

- Construct exporters, tracer providers, or agento11y clients in middleware.
- Add upstream telemetry registration, per-call recording controls, or agent, step, and tool span hierarchies.
- Change provider requests, provider responses, UI chunks, Server-Sent Event framing, or frontend behavior.
- Promise remote ingestion or per-generation delivery acknowledgement.

## Decisions

### Keep transport selection on the resolved client

The middleware continues to call `StartGeneration` or `StartStreamingGeneration`. Agento11y selects direct gRPC, direct HTTP, or OpenTelemetry export. Adding protocol or exporter fields to `WrapOptions` would duplicate client configuration and prevent per-request client resolution.

### Normalize provider names at the Agent Observability boundary

AI SDK keeps its upstream-compatible public identifiers. Before Agent Observability export, the middleware maps `amazon-bedrock` to `bedrock` and `anthropic.vertex` to `vertex`. Agento11y then emits the OpenTelemetry registry values `aws.bedrock` and `gcp.vertex_ai`. The hooks preflight span maps directly to those registry values because agento11y does not create that span.

### Mark only reported input totals as inclusive

The mapper sets `TokenInputSemanticsInclusive` when `InputTokens.Total` is non-nil, including a reported zero. An absent total leaves the agento11y usage value empty.

### Fail closed for changed or new transformed user messages

Agento11y v0.18 irreversibly converts unsupported string and numeric hook response roles to `user`. The middleware cannot distinguish a valid user rewrite from a demoted role. It therefore accepts each transformed user message only when it exactly matches one unused original user message. Changed, inserted, or duplicated user messages fail with `ErrHookTransformFailed`; omission and reordering remain valid.

This temporarily disables user-message redaction and rewriting. System-prompt, assistant-message, tool-message, and tool-list transformations remain available when user messages stay unchanged. Vendoring agento11y was rejected because it would add a large fork. Shipping the unsafe decoder was rejected because malformed policy output could alter the provider request.

### Keep OpenTelemetry lifecycle application-owned

Ending a generation span hands it to the configured span processor. `Client.Shutdown` invokes and waits for the configured `Flusher`, but success does not prove remote ingestion. The application shuts down the client before the tracer provider.

### Record field-level upstream differences

The generation span aligns with the upstream CLIENT `chat <model>` span and core request, response, usage, and content fields. Intentional differences remain:

- Routed calls replace request provider/model identity with complete backend response identity and retain wrapper identity in Agent Observability metadata. Upstream retains call-start identity and records the response model separately.
- Agent Observability maps `stop` to `end_turn`.
- Streaming timing uses `gen_ai.response.time_to_first_chunk`; upstream adds `gen_ai.client.operation.duration`, `gen_ai.client.operation.time_to_first_chunk`, and `gen_ai.client.operation.time_per_output_chunk`.
- Agent Observability adds generation IDs, parents, tags, metadata, token semantics, and normalized error categories.
- The Go mapper omits upstream request fields for frequency penalty, presence penalty, top-k, stop sequences, and seed.
- The Go integration has no upstream registration API, per-call input/output controls, or agent, step, and tool hierarchy.

## Risks / Trade-offs

- [User-message hook transforms are restricted] -> Reject them with a clear wrapped error until agento11y preserves unsupported roles, then remove the restriction in a coordinated dependency update.
- [Sampling or attribute limits can remove generation data] -> Document explicit sampling and content-capture ownership.
- [A successful flush can still precede remote loss] -> Define the contract at the client and span-processor boundary, not backend receipt.
- [Provider aliases differ from AI SDK public names] -> Normalize only inside this middleware and keep provider package identifiers unchanged.
- [Nested-module tidy depends on unreleased root code] -> Tidy with the repository root replacement and keep that existing limitation documented.
