# Prometheus metrics

Use `middleware/prometheus` for local metrics about provider calls: request
volume, failures, latency, in-flight calls, token usage, and stream timing.

These are provider-attempt metrics. Retries, fallback, and multi-step agents can
produce several observations for one application request.

## Install and create the collectors

```bash
go get github.com/grafana/ai-sdk/middleware/prometheus
```

```go
registry := prometheus.NewRegistry()

model, err := prommw.Wrap(baseModel, prommw.Options{
	Registerer: registry,
	ConstLabels: prometheus.Labels{
		"service": "chat-api",
	},
})
if err != nil {
	return err
}
```

Use an explicit Prometheus registry so collector ownership and duplicate
registration failures are clear. Construct one middleware per registerer and
reuse it. Use panic-on-startup helpers only when a registration error should
make process startup fail.

Attach the reusable middleware to a provider registry when every resolved model
should be measured.

## Expose the registry

```go
mux.Handle(
	"/metrics",
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
)
```

Protect and expose this endpoint according to your existing metrics policy.

## Understand what is measured

The middleware records:

- completed and in-flight provider calls;
- provider-call duration and outcome;
- input and output token totals;
- time to first stream output;
- optional stream-part counts and inter-chunk delay.

Streaming token metrics combine every usage-bearing part and preserve the
greatest value observed independently for each normalized counter.

`GenerateText` calls providers through the streaming path, so provider
middleware may observe it as a stream operation. Use orchestration-level
callbacks or separate HTTP metrics for one measurement of the entire user
request.

See the [package reference](https://pkg.go.dev/github.com/grafana/ai-sdk/middleware/prometheus)
for metric names, labels, and bucket options.

## Keep labels bounded

Prompts, outputs, tool data, user IDs, tenant IDs, request IDs, URLs, error
messages, and arbitrary metadata are deliberately excluded from labels.

Provider and model IDs can still be high-cardinality in gateway deployments.
Use `NormalizeProvider` and `NormalizeModel` to map dynamic identities to stable
families. Restrict constant labels to process-level dimensions such as service,
component, and environment.

## Tune stream volume

Stream-part metrics add work for each part. Disable chunk metrics when request,
latency, token, and time-to-first-output metrics are sufficient. Set histogram
buckets from your service-level objectives and observed provider latency.

## Choose middleware order

Place Prometheus outside policy middleware to measure all attempted calls,
including policy denials and middleware overhead. Place it inside to measure only
provider calls that passed policy. Document the choice so dashboards are
interpreted correctly.

This middleware records local client-side provider-call metrics. Remote service
metrics and controls are outside its scope.

## Reference

- [`middleware/prometheus`](https://pkg.go.dev/github.com/grafana/ai-sdk/middleware/prometheus)
- [Structured logging](structured-logging.md)
- [Production checklist](../best-practices/production.md)

---

← [Context enrichment](context-enrichment.md) · [Docs index](../README.md) · [Agent Observability →](agent-observability.md)
