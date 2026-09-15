# Text observability

The Gateway emits one logical observation for each authenticated unary or
streaming text-model call. Logs, Prometheus, and Agent Observability identify
the call as provider `grafana` and the canonical public catalog model ID. An
alias therefore has the same telemetry identity as its canonical model.

The fixed logical chain is request-observation context, Agent Observability
recording, structured logging, Prometheus, canonical identity, and then the
current lower model. The lower model is direct in WP8. It is an explicit seam
for WP9 fallback; physical candidates and attempts must remain below the
logical chain.

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
| `--agento11y.flush-interval` | `GRAFANA_AI_GATEWAY_AGENTO11Y_FLUSH_INTERVAL` | `1s`; positive, at most 5m |
| `--agento11y.flush-timeout` | `GRAFANA_AI_GATEWAY_AGENTO11Y_FLUSH_TIMEOUT` | `5s`; independent positive bound, at most 5m |
| `--agento11y.shutdown-timeout` | `GRAFANA_AI_GATEWAY_AGENTO11Y_SHUTDOWN_TIMEOUT` | `5s`; independent positive bound, at most 5m |

The secret reference is resolved once before the listener binds; neither its
name nor value is logged. Ambient `AGENTO11Y_*` and legacy `SIGIL_*` SDK
configuration is rejected so it cannot bypass validated Gateway policy.

Queue pressure, validation failures, exporter rejection/outage, and bounded
flush or shutdown failure are fail-open for model traffic. Diagnostics and
metrics expose only a fixed failure class. After readiness falls and HTTP
serving stops, the process first waits up to the flush-timeout budget for
already-started recorders and refuses new recorder acquisition. Flush and
shutdown then each receive a fresh independent timeout. The
`process_shutdown_completed` lifecycle event is logged only after this bounded
finalizer returns.

## Work-package boundaries

- WP6 image capacity/distribution does not use this text-model chain.
- WP7's Go client and ProviderWire contract are unchanged; correlation is
  server-internal and is neither accepted from nor returned to clients.
- WP9 owns physical fallback attempts, candidate identity, and retry topology
  below the unchanged logical wrapper.
- WP10 owns production endpoint, credential, region/application values,
  rollout, and environment smoke verification.
- WP27 owns any later per-request Agent Observability control or richer content
  capture decision.
- Tools, reasoning, files, images, raw output, hooks, and later event families
  remain with their owning capability work; WP8 observes the current text
  surface only and does not change ProviderWire schemas or events.
