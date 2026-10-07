# Text observability

The Gateway emits one logical observation for each authenticated unary or
streaming text-model call. Logs, Prometheus, and Agent Observability identify
the call as provider `grafana` and the canonical public catalog model ID. An
alias therefore has the same telemetry identity as its canonical model.

A request produces one logical observation even when fallback tries several
models. Use the public model ID to monitor application traffic and private
fallback logs to investigate individual provider attempts.

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

Use private stderr logs to investigate which models were attempted for a request.
Each `gateway_physical_attempt` record includes the request correlation ID when
available, candidate index, configured provider and backend model, timing,
outcome, and whether fallback was planned. These records are separate from the
logical generation observation and exclude payloads, credentials, headers,
endpoint URLs, and raw errors. Restrict access because they reveal provider
configuration that is not exposed to callers.

Verify your runtime's stderr transport before relying on these diagnostics.
Supported destinations are Linux sockets and pipes, plus sockets already
configured as nonblocking on other Unix platforms. Blocking macOS sockets,
ordinary files, and terminals disable this output without interrupting model
calls. The Gateway does not change the shared stderr descriptor's flags.

Attempt logging uses a 256-record queue, a 100 ms write deadline, a 4096-byte
record limit, and a one-second shutdown budget. Queue saturation or write failures
can drop records; monitor `grafana_ai_gateway_physical_attempt_dropped_total`
before treating the logs as a complete account of provider attempts.

## Prometheus

The existing unauthenticated `GET /metrics` endpoint is the single scrape
target for both HTTP and model metrics. Model collectors are registered once
on the service-owned registry before listener binding; duplicate registration
fails startup.

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

Production requires Agent Observability to be enabled, TLS-protected, and
credentialed. Development may disable it explicitly. Every setting has the
equivalent `GRAFANA_AI_GATEWAY_` environment binding shown below.

| Flag | Environment | Default / constraint |
| --- | --- | --- |
| `--observation.region` | `GRAFANA_AI_GATEWAY_OBSERVATION_REGION` | optional, visible ASCII, at most 128 bytes |
| `--observation.application` | `GRAFANA_AI_GATEWAY_OBSERVATION_APPLICATION` | optional, visible ASCII, at most 128 bytes |
| `--agento11y.enabled` | `GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED` | `false`; required in production |
| `--agento11y.protocol` | `GRAFANA_AI_GATEWAY_AGENTO11Y_PROTOCOL` | `grpc`; `grpc` or `http` |
| `--agento11y.endpoint` | `GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT` | required when enabled; credential-free absolute HTTP(S) URL for HTTP, host:port for gRPC |
| `--agento11y.tls` | `GRAFANA_AI_GATEWAY_AGENTO11Y_TLS` | `true`; required in production |
| `--agento11y.auth-secret-env` | `GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV` | required environment-variable reference; SDK-reserved names are rejected |
| `--agento11y.queue-size` | `GRAFANA_AI_GATEWAY_AGENTO11Y_QUEUE_SIZE` | `2000`; 1–1,000,000 |
| `--agento11y.batch-size` | `GRAFANA_AI_GATEWAY_AGENTO11Y_BATCH_SIZE` | `100`; 1–queue size |
| `--agento11y.payload-max-bytes` | `GRAFANA_AI_GATEWAY_AGENTO11Y_PAYLOAD_MAX_BYTES` | 16 MiB; at most 64 MiB |
| `--agento11y.max-retries` | `GRAFANA_AI_GATEWAY_AGENTO11Y_MAX_RETRIES` | `5`; 1–10 |
| `--agento11y.initial-backoff` | `GRAFANA_AI_GATEWAY_AGENTO11Y_INITIAL_BACKOFF` | `100ms`; positive, at most 5m |
| `--agento11y.max-backoff` | `GRAFANA_AI_GATEWAY_AGENTO11Y_MAX_BACKOFF` | `5s`; initial–5m |
| `--agento11y.export-timeout` | `GRAFANA_AI_GATEWAY_AGENTO11Y_EXPORT_TIMEOUT` | `10s`; per HTTP/gRPC export attempt, positive, at most 5m |
| `--agento11y.flush-interval` | `GRAFANA_AI_GATEWAY_AGENTO11Y_FLUSH_INTERVAL` | `1s`; positive, at most 5m |
| `--agento11y.flush-timeout` | `GRAFANA_AI_GATEWAY_AGENTO11Y_FLUSH_TIMEOUT` | `5s`; independent positive bound, at most 5m |
| `--agento11y.shutdown-timeout` | `GRAFANA_AI_GATEWAY_AGENTO11Y_SHUTDOWN_TIMEOUT` | `5s`; independent positive bound, at most 5m |

The secret reference is resolved once before the listener binds; neither its
name nor value is logged. Ambient `AGENTO11Y_*` and legacy `SIGIL_*` SDK
configuration is rejected so it cannot bypass validated Gateway policy.

Use `--agento11y.export-timeout` to bound each export attempt independently of
flush and shutdown timeouts.

Queue pressure, validation failures, exporter rejection/outage, and bounded
flush or shutdown failure are fail-open for model traffic. Diagnostics and
metrics expose only a fixed failure class. After readiness falls and HTTP
serving stops, the process first waits up to the flush-timeout budget for
already-started recorders and refuses new recorder acquisition. Flush and
shutdown then each receive a fresh independent timeout. The
`process_shutdown_completed` lifecycle event is logged only after this bounded
finalizer returns.

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
