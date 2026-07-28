// Package enrichment provides opt-in provider-call context enrichment middleware.
//
// The middleware projects explicitly approved server-side request context into
// provider-bound metadata channels: provider.CallOptions.Headers and
// provider.CallOptions.ProviderOptions. It never mutates prompts, messages,
// tools, tool arguments, stream parts, UI chunks, provider metadata, or
// telemetry. The package is provider-agnostic and intentionally does not import
// provider modules.
//
// Enrichment is default-deny. Values are collected only from Options.Values,
// context helpers when Options.ContextValues is true, and Options.DynamicValues.
// Collected values are string-only and are emitted only when selected by
// Options.Filter.Include or by a destination-specific map. Header mappings are
// visible only to the header output, and provider-options mappings are visible
// only to the provider-options output.
//
// Basic header enrichment:
//
//	model = enrichment.Wrap(model, enrichment.Options{
//		Values: []enrichment.Value{{Key: "service", Value: "api"}},
//		Headers: enrichment.HeaderOptions{
//			Map: map[string]string{"service": "X-AI-Service"},
//		},
//	})
//
// Request-derived values can be supplied through context helpers or
// Options.DynamicValues:
//
//	ctx = enrichment.WithValue(ctx, "request_id", requestID,
//		enrichment.WithCardinality(enrichment.CardinalityHigh))
//	model = enrichment.Wrap(model, enrichment.Options{
//		ContextValues: true,
//		Headers: enrichment.HeaderOptions{
//			Map: map[string]string{"request_id": "X-Request-Id"},
//		},
//	})
//
//	model = enrichment.Wrap(model, enrichment.Options{
//		DynamicValues: func(ctx context.Context, input enrichment.CallInput) ([]enrichment.Value, error) {
//			return []enrichment.Value{{Key: "tenant", Value: tenantFromRequest(ctx)}}, nil
//		},
//		Headers: enrichment.HeaderOptions{
//			Map: map[string]string{"tenant": "X-Tenant"},
//		},
//	})
//
// For Grafana hosted provider request context, prefer provider options rather
// than hosted middleware control headers:
//
//	model = enrichment.Wrap(model, enrichment.Options{
//		ContextValues: true,
//		Filter: enrichment.FilterOptions{Include: []string{"request_id", "tenant"}},
//		ProviderOptions: enrichment.ProviderOptionsConfig{
//			ProviderKey: "grafana",
//			ObjectKey:   "enrichment",
//		},
//	})
//
// This writes a grafana.enrichment sidecar while preserving unrelated Grafana
// hosted controls such as grafana.agentObservability, grafana.tracing,
// grafana.metrics, and grafana.usage.
//
// Do not propagate secrets, API tokens, raw auth claims, prompts, tool
// arguments, raw user input, or values intended for metric labels. Built-in
// protected auth and transport headers are never written or overwritten by
// enrichment. High-cardinality values such as request IDs can be useful for
// correlation headers or provider options, but should not be reused as metric
// labels.
//
// Middleware order remains the core middleware order: the first middleware is
// outermost and transforms params first. Put enrichment before Agent
// Observability when hooks and recording should see enriched CallOptions; put it
// after Agent Observability when enrichment is transport-only from its
// perspective. Use Agent Observability's context helpers for generation DAG
// metadata.
package enrichment
