# Text observability

The Gateway emits one logical observation for each authenticated unary or
streaming text-model call. Logs, Prometheus, and Agent Observability identify
the call as provider `grafana` and the canonical public catalog model ID. An
alias therefore has the same telemetry identity as its canonical model.

A request produces one logical observation even when fallback tries several
models. Use the public model ID to monitor application traffic and private
fallback logs to investigate individual provider attempts.

## Logs and traces through the plugin host

The Gateway writes every log record through the plugin SDK logger, which the
plugin host relays as structured records at their own level. Nested attributes
appear as dotted keys, and records stay below the host's 64 KiB line limit. The
plugin writes nothing else to its standard streams.

Tracing comes from the plugin host. When the host configures it through
`GF_INSTANCE_OTLP_*`, the plugin SDK instruments the route call and exports
spans; the Gateway adds no exporter or span attributes of its own. Request log
records carry the `traceID` and `spanID` of the incoming trace context.

## Trusted metadata and privacy

The Gateway generates a new bounded correlation ID for the HTTP request. Only
these additional values may enter logical logs and Agent Observability
metadata:

| Field | Trusted source |
| --- | --- |
| `gateway.correlation_id` | Gateway-generated opaque request ID |
| `gateway.caller_service` | successfully verified service identity |
| `gateway.namespace` | successfully verified authentication namespace |
| `gateway.region` | validated static `observation.region` setting |
| `gateway.application` | validated static `observation.application` setting |

Missing values are omitted. Incoming headers, raw claims, acting-user values,
arbitrary context, and provider response metadata are not trusted sources.
Observation metadata is never copied into provider headers or provider
options, and none of these request-scoped fields becomes a metric label.

Model logging disables prompt, output, and per-stream-part capture and applies
a fixed attribute allowlist followed by the default secret redactor. Agent
Observability always uses metadata-only content capture, provided-only context,
requested identity, a final Gateway allowlist, and no hooks. It retains
lifecycle, structural part types, normalized usage and unified finish/error
classes, but not text, detailed errors, credentials, response IDs, backend
model IDs, provider topology, arbitrary tags, or request controls. The client
may add its fixed `agento11y.sdk.*` metadata markers and a closed `call_error`
category after the Gateway filter; no arbitrary exporter or provider error is
exported.

## Inspect fallback attempts

Use the Gateway's relayed logs to investigate which models were attempted for a request.
Each `gateway_physical_attempt` record includes the request correlation ID when
available, candidate index, configured provider and backend model, timing,
outcome, and whether fallback was planned. These records are separate from the
logical generation observation and exclude payloads, credentials, headers,
endpoint URLs, and raw errors. Restrict access because they reveal provider
configuration. Callers may independently receive compact execution overviews;
operator timing, correlation and fallback-intent records are not consumer output.

Attempt records go through the same logger from one background worker with a
256-record queue, so model calls never wait for log output. Provider and
model identities in a record are capped at 512 bytes each. Queue saturation, invalid records and records emitted after
shutdown are dropped; monitor `grafana_ai_gateway_physical_attempt_dropped_total`
before treating the logs as a complete account of provider attempts.

## Caller execution overviews

Configured direct and fallback routes return best-effort `gateway.execution`
metadata on unary results, stream finishes and failure carriers. It contains
requested/canonical identity and ordered observed attempts, not a trace or
completion/replay claim. Current committed error summaries remain event-local
and do not require configured catalog attribution. The handler replaces exact
configured provider-key and enabled exporter-credential matches in summary
message, type and string code for all callers. Static Gateway access keys remain
outside this policy and retain hash-only authentication storage. Other provider-originated values, including BYOK-only echoes, remain
unchanged. This policy is independent of logger field redaction and does not
scan native metadata, generated text or stream fragments.

Execution metadata is added directly to responses, not reconstructed from
Gateway logs. Disabling telemetry or dropping log records does not suppress it.
If your application records this metadata, apply its own access and retention
controls. See
[consumer access and omission semantics](../../docs/providers/grafana-gateway.md#inspect-execution-overviews-and-failures).

## Prometheus

The plugin host collects the Gateway's metrics through the plugin protocol; the
Gateway serves no metrics endpoint. Gateway collectors are registered once on
the plugin's default registry when the plugin starts, and duplicate registration
fails startup. Route metrics use the fixed route names `config`, `language_model`
and `unmatched`, never namespaces.
`grafana_ai_gateway_access_decisions_total` counts route requests by the fixed
`decision` values `configured`, `byok` and `refused`.

The `aisdk_model_*` families use only the following bounded labels:

| Family | Labels |
| --- | --- |
| `requests_total` | operation, provider, model, status, error type, status code, finish reason |
| `inflight_requests` | operation, provider, model |
| `request_duration_seconds` | operation, provider, model, status |
| `tokens_total` | operation, provider, model, token type |
| `time_to_first_output_seconds` | operation, provider, model, status |
| `stream_chunks_total` | operation, provider, model, closed chunk type |
| `inter_chunk_delay_seconds` | operation, provider, model, closed chunk type |

Provider is always `grafana`, model is always the canonical public ID, and
operation/status/error/finish/chunk values come from finite classifications;
unknown stream-part values are bucketed as `other`.
Status-code labels use `100`–`599`, `none` when absent, and `other` for every
out-of-range provider value.
Caller, namespace, correlation, region, application, aliases, backend identity,
URLs, and content are never labels. `grafana_ai_gateway_agento11y_export_failures_total`
uses only the fixed `class` values `queue_full`, `serialization`, `transport`,
`rejected`, `shutdown`, and `unknown`.

## Agent Observability configuration

Agent Observability export is optional in every deployment mode. When enabled
in production, it must be TLS-protected and credentialed. The plugin reads these
settings from its environment.

| Environment | Default / constraint |
| --- | --- |
| `GRAFANA_AI_GATEWAY_OBSERVATION_REGION` | optional, visible ASCII, at most 128 bytes |
| `GRAFANA_AI_GATEWAY_OBSERVATION_APPLICATION` | optional, visible ASCII, at most 128 bytes |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED` | `false` |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_PROTOCOL` | `grpc`; `grpc` or `http` |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT` | required when enabled; credential-free absolute HTTP(S) URL for HTTP, host:port for gRPC |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_TLS` | `true`; required in production when enabled |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV` | required environment-variable reference; SDK-reserved names are rejected |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_QUEUE_SIZE` | `2000`; 1–1,000,000 |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_BATCH_SIZE` | `100`; 1–queue size |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_PAYLOAD_MAX_BYTES` | 16 MiB; at most 64 MiB |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_MAX_RETRIES` | `5`; 1–10 |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_INITIAL_BACKOFF` | `100ms`; positive, at most 5m |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_MAX_BACKOFF` | `5s`; initial–5m |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_EXPORT_TIMEOUT` | `10s`; per HTTP/gRPC export attempt, positive, at most 5m |
| `GRAFANA_AI_GATEWAY_AGENTO11Y_FLUSH_INTERVAL` | `1s`; positive, at most 5m |

The secret reference is resolved once while the plugin starts; neither its
name nor value is logged. Ambient `AGENTO11Y_*` and legacy `SIGIL_*` SDK
configuration is rejected so it cannot bypass validated Gateway policy.

Use `GRAFANA_AI_GATEWAY_AGENTO11Y_EXPORT_TIMEOUT` to bound each export attempt.

Queue pressure, validation failures, exporter rejection/outage, and bounded
flush or shutdown failure are fail-open for model traffic. Diagnostics and
metrics expose only a fixed failure class.

After the plugin host stops the plugin, the Gateway flushes Agent Observability
and attempt records concurrently within one second. Agent Observability waits
for already-started recorders, flushes and shuts down, all within that shared
deadline. Records emitted after the flush
starts are dropped and counted. The host's tracer shutdown shares the same
window before the host kills the process, so a slow trace exporter can cut the
flush short. The `plugin_stopped` lifecycle event is logged after the flush.

## Returned values and consumer observation

Gateway telemetry helps you monitor usage, latency and failures without
collecting response content. Your application can still receive model warnings,
citations and response details; making those available to the caller does not
add them to Gateway logs or metrics.

The model named in a response may differ from the public model you selected.
Gateway telemetry continues to identify calls by their configured public model,
so aliases share the same operational view. Use response details for inspecting
a particular generation, not as a replacement for the model ID your application
uses to select a model. See the [Go client guide](../../docs/providers/grafana-gateway.md#native-response-values)
for accessing those details in generated and streamed responses.

If you need response content for application diagnostics, configure logging in
your application separately. This does not enable content capture on the
Gateway. Capture only what you need, restrict access and retention, and account
for sensitive content in warnings, citations and response bodies. The
[structured logging guide](../../docs/middleware/structured-logging.md)
explains how to choose capture settings and redact application logs.

## Source and tool privacy

Sources and tool calls remain available in model responses, but their content
is not captured in logs, metrics or Agent Observability. Source identifiers,
URLs, titles, filenames and metadata are omitted, as are tool names, IDs,
arguments, results and provider metadata. Returning provider metadata to an
application does not authorize telemetry capture.

See the [source guide](sources.md) for the public response privacy policy.
