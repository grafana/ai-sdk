# Monitor the Gateway

The Gateway reports every request through Grafana's logs, metrics and traces,
and can also export model calls to Agent Observability. It records which model
was called, how the call went, how long it took and how many tokens it used.
It never records prompts, responses or credentials.

## Logs

Gateway logs appear in Grafana's log with `logger=plugin.grafana-aigateway-app`.
Each request writes one `http request completed` line:

| Field | Value |
|---|---|
| `route` | `config` for model listing, `language_model` for model calls |
| `status` | Status class, such as `2xx` or `4xx` |
| `access` | `configured`, `byok` (bring your own key) or `refused` |
| `correlation_id` | ID that links the request to its attempt logs |

Each model the Gateway tries for a request writes a `gateway_physical_attempt`
line with the same `correlation_id`, the provider and model it tried, the
outcome and whether a fallback followed. Use these lines to see which provider
answered a request. They reveal your provider configuration, so limit who can
read them.

Under heavy load the Gateway drops attempt lines rather than slow down model
calls. `grafana_ai_gateway_physical_attempt_dropped_total` counts the dropped
lines.

## Metrics

Grafana serves the Gateway's metrics at
`/metrics/plugins/grafana-aigateway-app`. Model metrics use the model ID that
applications select, so calls through an alias count under their model.

| Metric | Measures |
|---|---|
| `aisdk_model_requests_total` | Model calls, by model, status, error type and finish reason |
| `aisdk_model_request_duration_seconds` | Model call duration |
| `aisdk_model_time_to_first_output_seconds` | Time until the model starts answering |
| `aisdk_model_tokens_total` | Tokens, by model and token type |
| `aisdk_model_inflight_requests` | Model calls in progress |
| `aisdk_model_stream_chunks_total` | Streamed parts, by type |
| `aisdk_model_inter_chunk_delay_seconds` | Time between streamed parts |
| `grafana_ai_gateway_access_decisions_total` | Requests, by access decision |
| `grafana_ai_gateway_agento11y_export_failures_total` | Failed Agent Observability exports, by cause |

Grafana's own request metrics, such as `grafana_router_http_requests_total`,
count requests to the Gateway and their latency.

## Traces

When Grafana exports traces, it also traces requests to the Gateway, and the
Gateway's log lines include their `traceID` and `spanID`.

## Export to Agent Observability

The Gateway can send each model call to Agent Observability. Add these settings to the `[plugin.grafana-aigateway-app]` section:

```ini
agento11y_enabled = true
agento11y_endpoint = otlp.example.com:443
agento11y_token = $__file{/run/secrets/agento11y-token}
```

| Setting | Description |
|---|---|
| `agento11y_enabled` | `true` to export. Defaults to `false`. |
| `agento11y_protocol` | `grpc` (default) or `http`. |
| `agento11y_endpoint` | Collector address: `host:port` for gRPC, or a URL for HTTP. |
| `agento11y_tls` | Defaults to `true`. Set `false` only for a collector without TLS. |
| `agento11y_token` | Token for the collector. Required when exporting. |

The export includes the model, timing, token usage and how the call ended, but
no content. When one stream carries several messages or responses, one export
covers all of them, with the last finish reason and the last message's usage,
the same usage that logs and metrics report. If a stream stalls or is canceled
after a message finished, the export records the finished messages without a
timeout or cancellation error; logs and metrics still record the timeout or
cancellation. If the collector is slow or unavailable, model calls continue and
the export failure metric counts the lost records.

Configure the export only through these settings. If Grafana's environment
has `AGENTO11Y_*` or `SIGIL_*` variables, the Gateway does not start.

## What the Gateway does not record

The Gateway's logs, metrics and exports never include:

- prompts, responses, generated files, tool calls or citations;
- provider keys, Grafana tokens or request headers;
- user or service account IDs.

In `cloud` mode, records of configured-model calls also name the Grafana
service that made the call.

Your application still receives everything the model returns, including
warnings, citations and response details. If you log those in your
application, apply your own access and retention rules; the
[structured logging guide](../../docs/middleware/structured-logging.md) shows
how to capture and redact them.

---

← [Sources](sources.md) · [Docs index](../../docs/README.md) · [AI Gateway](../README.md)
