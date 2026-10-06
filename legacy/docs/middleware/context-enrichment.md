# Context enrichment

Use `middleware/enrichment` to send a small, approved set of server-side request
attributes to a provider or gateway. Typical values are request correlation,
region, plan, or tenant-routing metadata.

Do not use enrichment to add prompt content, arbitrary context values, secrets,
or metric labels.

## Install and choose a destination

```bash
go get github.com/grafana/ai-sdk/middleware/enrichment
```

Enrichment can write to:

- request headers, when the provider or gateway expects transport metadata;
- provider options, when the receiving service expects structured metadata.

Use provider options for structured application context and explicit header maps
for protocol-defined headers.

## Add a static value

```go
model := enrichment.Wrap(baseModel, enrichment.Options{
	Values: []enrichment.Value{
		{Key: "service", Value: "chat-api"},
	},
	Headers: enrichment.HeaderOptions{
		Map: map[string]string{"service": "X-AI-Service"},
	},
})
```

The wrapped model works anywhere the original model did.

## Add request-scoped values

Only values added with the enrichment context helpers are read:

```go
ctx = enrichment.WithValue(
	ctx,
	"request_id",
	requestID,
	enrichment.WithCardinality(enrichment.CardinalityHigh),
)

model := enrichment.Wrap(baseModel, enrichment.Options{
	ContextValues: true,
	Headers: enrichment.HeaderOptions{
		Map: map[string]string{"request_id": "X-Request-Id"},
	},
})
```

The middleware does not inspect arbitrary `context.Context` keys. Use
`DynamicValues` when values must be computed from the current call; decide
whether a lookup failure should fail the model call or be handled by `OnError`.

## Use default-deny selection

A collected value is emitted only when selected by an include list or an
explicit destination map. Exclusions win. Keep the allowlist small and specific
to what the receiver consumes.

```go
Filter: enrichment.FilterOptions{
	Include:         []string{"tenant", "region"},
	Exclude:         []string{"debug_token"},
	RedactSensitive: true,
	MaxValueLength:  256,
}
```

Use cardinality metadata to prevent request IDs and other unbounded values from
reaching destinations intended for dimensions. Marking a value sensitive only
helps when redaction is enabled; it does not make the value safe to transmit.

## Protect headers

Authentication and transport-control headers are protected and cannot be
written by enrichment. Caller values win conflicts by default. Add any
application-specific credentials to the protected set, and avoid broad prefix
mapping when an explicit map will do.

Never propagate API tokens, raw auth claims, prompts, tool arguments, or raw user
input through enrichment.

## Choose middleware order

Place enrichment outside another middleware when that middleware should see the
enriched provider call. Place it inside when enrichment is transport-only from
the outer middleware's perspective. Use Agent Observability's context helpers for
generation relationships.

## Reference

- [`middleware/enrichment`](https://pkg.go.dev/github.com/grafana/ai-sdk/middleware/enrichment)
- [Middleware overview](overview.md)
- [Grafana Cloud provider](../providers/grafana-cloud.md)

---

← [Structured logging](structured-logging.md) · [Docs index](../README.md) · [Prometheus metrics →](prometheus.md)
