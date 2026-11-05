## Context

`RecordingMiddleware` already delegates generation creation to the resolved agento11y client and passes the returned context to the provider. The middleware module already pins agento11y v0.19.0, whose OpenTelemetry generation protocol uses this existing path.

The dependency also has compatibility constraints. The v0.19.0 hook decoder accepts numeric protobuf roles, but converts unsupported response roles to `user` and drops malformed parts before middleware validation. The dependency exposes no raw response API. No patched agento11y release exists.

The registered Vercel baseline is commit `612be2311464804d1e4897afacbf8f3fc2f6d9e3`, with `ai@7.0.153` and `@ai-sdk/provider@4.0.34`. `@ai-sdk/otel` is not registered. The span comparison uses `@ai-sdk/otel@1.0.153`, which depends on exactly those registered versions. Agent Observability is a Grafana-specific integration rather than a complete port of the upstream telemetry API.

## Goals / Non-Goals

**Goals:**

- Support client-selected agento11y v0.19.0 OpenTelemetry generation export without a middleware-owned transport API.
- Preserve provider results, stream parts, context values, and generation lifetime.
- Export standard OpenTelemetry provider names for the built-in Bedrock and Anthropic Vertex models.
- Reject every non-nil decoded hook transform before provider invocation until decoding preserves invalid data for validation or rejects it.
- Record application ownership and field-level upstream differences.

**Non-Goals:**

- Construct exporters, tracer providers, or agento11y clients in middleware.
- Add upstream telemetry registration, per-call recording controls, or agent, step, and tool span hierarchies.
- Change provider requests, provider responses, UI chunks, Server-Sent Event framing, or frontend behavior.
- Promise remote ingestion or per-generation delivery acknowledgement.
- Register `@ai-sdk/otel` in `test/conformance/upstream.yaml`.

## Decisions

### Keep transport selection on the resolved client

The middleware continues to call `StartGeneration` or `StartStreamingGeneration`. Agento11y selects direct gRPC, direct HTTP, or OpenTelemetry export. Adding protocol or exporter fields to `WrapOptions` would duplicate client configuration and prevent per-request client resolution.

### Normalize provider names at the Agent Observability boundary

AI SDK keeps its upstream-compatible public identifiers. Before Agent Observability export, the middleware maps `amazon-bedrock` to `bedrock` and `anthropic.vertex` to `vertex`, and hook requests use the same names. Agento11y then emits the OpenTelemetry registry values `aws.bedrock` and `gcp.vertex_ai`. The hooks preflight span uses a copy of agento11y's stored-to-registry table because agento11y does not create that span. Transport metadata keeps the AI SDK name, as the logger middleware does.

This changes stored provider values for every export protocol. Hook rules and queries that match `amazon-bedrock` or `anthropic.vertex` must use the stored names.

### Temporarily reject every decoded transform

Agento11y v0.19.0 changes unsupported roles to `user` and drops malformed parts before middleware validation, so valid decoded content does not prove the response was lossless. There is no raw response API for middleware to validate the discarded data.

`HooksMiddleware` rejects every non-nil decoded `TransformedInput` with `ErrHookTransformFailed` before provider invocation, regardless of `FailOpen`. This applies to generate and stream calls. Valid assistant, system-prompt, tool-message, tool-list, and user changes are rejected, including replacements that leave user messages unchanged. Even a fully unchanged replacement is rejected. In the standard stack, rejected transforms also bypass recording.

An empty HTTP `transformed_input:{}` is not a decoded transform: the dependency normalizes it to nil. Allow without a decoded transform passes the original request through. Deny behavior stays unchanged.

The restriction is temporary until the decoder preserves invalid data for validation or rejects it. Lossless reconstruction rules remain in the specification as inactive constraints for that future change. `applyTransformedRequest` keeps the prompt, tool, and tool-choice reconstruction in one helper that the middleware does not call. Helper tests and fixtures do not establish active middleware support. Successful-transform telemetry scenarios are also inactive.

Vendoring agento11y was rejected because it would add a large fork. Allowing selected transforms was rejected because middleware cannot detect data that the decoder discarded.

### Keep OpenTelemetry lifecycle application-owned

Ending a generation span hands it to the configured span processor. `Client.Shutdown` invokes and waits for the configured `Flusher`, but success does not prove remote ingestion. The application shuts down the client before the tracer provider.

### Preserve consumer recording controls

The recording path retains requested identity, provided-only context, finite cancellation cleanup, and lifecycle callbacks. `GenerationStart` includes only identity and context fields, so a generation filter decides which request and response fields reach the final generation. Agento11y restores context fields from the start when the filter omits them. Immediate provider errors also pass mapped request data through the filter. Agento11y never exposes request controls to samplers.

### Record field-level upstream differences

The main specification lists the differences between the agento11y `chat <model>` span and the `@ai-sdk/otel@1.0.153` span under "Generation spans follow the resolved client protocol": routed identity, finish reasons, the `anthropic.vertex` registry name, client operation timing attributes, omitted request penalties, top-k and stop sequences, Agent Observability additions, and the absent registration API, recording controls, and span hierarchy.

## Risks / Trade-offs

- All hook transforms are temporarily unavailable, including valid rewrites and tool changes. Re-enable them only with a decoder that preserves or rejects invalid data.
- [Sampling or attribute limits can remove generation data] -> Document explicit sampling and content-capture ownership.
- [A successful flush can still precede remote loss] -> Define the contract at the client and span-processor boundary, not backend receipt.
- [Provider aliases differ from AI SDK public names] -> Normalize only inside this middleware and keep provider package identifiers unchanged.
- [Stored provider values change for existing users] -> Mark the change as breaking and document the stored names.
- [The span comparison is not enforced] -> `@ai-sdk/otel` is not registered, so a baseline upgrade must recheck the listed differences by hand.
