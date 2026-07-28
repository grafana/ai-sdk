# Middleware

Middleware adds behavior such as defaults, logging, metrics, request metadata,
or policy checks to every model call. Configure the behavior once instead of
repeating it in each handler, job, or agent.

## Choose an integration

| You need | Use |
|---|---|
| Default settings, reasoning extraction, simulated streaming, or custom transforms | [Core middleware](#built-in-middleware) |
| Structured provider-call records in `log/slog` | [Structured logging](structured-logging.md) |
| Locally collected call, latency, stream, and token metrics | [Prometheus metrics](prometheus.md) |
| Approved request metadata sent to a provider or gateway | [Context enrichment](context-enrichment.md) |
| Generation investigation in Grafana and optional preflight policy | [Agent Observability](agent-observability.md) |

Middleware wraps a `provider.LanguageModel` and returns another model that works
with the same generation and streaming APIs.

## Wrap a model

```go
model := middleware.WrapLanguageModel(
	baseModel,
	middleware.DefaultSettings(defaults),
	middleware.ExtractReasoning(reasoningOptions),
)
```

The returned model implements the same interface as the original, so it works
with generation, agents, fallback, registries, and catalogs.

Use `middleware.Wrap` when you also need to override the public provider or
model identity of the wrapper.

## Understand ordering

The first middleware is the outermost layer. It sees the call first and the
result last:

```text
request  → first → second → provider
response ← first ← second ← provider
```

Ordering matters when one middleware transforms parameters that another logs,
records, or evaluates. Decide whether observability should capture the caller's
request, the transformed provider request, or both.

## Built-in middleware

The root middleware package includes lightweight behavior:

- default model settings;
- simulated streaming for non-streaming models;
- extraction of tagged reasoning from text;
- custom stream transformation.

See the [middleware API reference](https://pkg.go.dev/github.com/grafana/ai-sdk/middleware)
for constructors and exact options.

## Optional production middleware

Install these separate modules only when needed:

- [Structured logging](structured-logging.md) for provider-call lifecycle records
- [Prometheus metrics](prometheus.md) for local provider-call metrics
- [Context enrichment](context-enrichment.md) for approved provider-bound metadata
- [Agent Observability](agent-observability.md) for generation recording and policy hooks

Attach middleware at the registry boundary when every resolved model should use
the same policy. Construct stateful middleware, such as Prometheus collectors,
once and reuse it across requests.

## Choose what to observe

Middleware runs around each provider model call. A multi-step agent, retries,
and fallback can trigger it several times for one application request. Use
orchestration callbacks for one event covering the complete operation and model
middleware for each provider attempt.

Custom middleware must preserve cancellation, stream order, and errors.

## Reference

- [`middleware` package](https://pkg.go.dev/github.com/grafana/ai-sdk/middleware)
- [`registry.WithLanguageModelMiddleware`](https://pkg.go.dev/github.com/grafana/ai-sdk/registry#WithLanguageModelMiddleware)

---

← [Fallback and registry](../guides/fallback-and-registry.md) · [Docs index](../README.md) · [Structured logging →](structured-logging.md)
