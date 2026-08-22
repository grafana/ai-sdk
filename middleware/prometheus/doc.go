// Package prometheus provides provider-level Prometheus metrics middleware for
// ai-sdk language models.
//
// It is a nested Go module under middleware/prometheus/ so the Prometheus
// client dependency is only added for consumers who explicitly import this
// package. The root github.com/grafana/ai-sdk module does not import this
// package.
//
// # API
//
// Construct a reusable middleware once per Prometheus registerer and reuse it
// for direct wrapping or registry-wide wrapping:
//
//	reg := prometheus.NewRegistry()
//	mw, err := prommw.Middleware(prommw.Options{Registerer: reg})
//	if err != nil {
//		return err
//	}
//	model := middleware.Wrap(middleware.WrapOptions{
//		Model:      base,
//		Middleware: []middleware.Middleware{mw},
//	})
//
// Wrap is a convenience helper for single-model use:
//
//	model, err := prommw.Wrap(base, prommw.Options{Registerer: reg})
//
// For registry-wide instrumentation, pass the constructed middleware to the
// registry:
//
//	mw, err := prommw.Middleware(prommw.Options{Registerer: reg})
//	if err != nil {
//		return err
//	}
//	models := registry.NewProviderRegistry(providers,
//		registry.WithLanguageModelMiddleware(mw),
//	)
//
// Middleware and Wrap return Prometheus registration errors, including
// duplicate collector registration. MustMiddleware and MustWrap panic on those
// errors for examples/tests and applications that prefer panic-on-startup
// behavior. The package does not use promauto internally.
//
// # Metric contract
//
// The package registers these metric families:
//
//   - aisdk_model_requests_total: counter labeled by operation, provider, model,
//     status, error_type, status_code, and finish_reason.
//   - aisdk_model_inflight_requests: gauge labeled by operation, provider, and
//     model.
//   - aisdk_model_request_duration_seconds: histogram labeled by operation,
//     provider, model, and status.
//   - aisdk_model_tokens_total: counter labeled by operation, provider, model,
//     and token_type.
//   - aisdk_model_time_to_first_output_seconds: stream histogram labeled by
//     operation, provider, model, and status.
//   - aisdk_model_stream_chunks_total: stream counter labeled by operation,
//     provider, model, and chunk_type.
//   - aisdk_model_inter_chunk_delay_seconds: stream histogram labeled by
//     operation, provider, model, and chunk_type.
//
// Histogram buckets use seconds. Options can override request duration,
// time-to-first-output, and inter-chunk-delay buckets. Setting
// DisableStreamChunkMetrics disables aisdk_model_stream_chunks_total and
// aisdk_model_inter_chunk_delay_seconds while preserving request, duration,
// in-flight, token, and time-to-first-output metrics.
//
// # Provider-call scope
//
// The middleware instruments provider.LanguageModel.DoGenerate and DoStream.
// It records provider attempts, not logical StreamText or GenerateText
// operations. Retries, fallback, and multi-step orchestration can therefore
// produce multiple observations. GenerateText currently uses streaming
// internally, so it normally appears to provider middleware as stream calls.
//
// # Stream behavior
//
// Stream instrumentation tees provider.StreamPart values without mutating,
// dropping, or reordering them. Metrics finalize when the upstream stream
// closes or the request context is canceled. Callers must drain returned
// streams or cancel their contexts to let stream metrics finalize and avoid
// goroutine leaks.
//
// # Identity, privacy, and cardinality
//
// By default, final metrics prefer response provider/model metadata when both
// are available, falling back to requested model identity. In-flight gauges use
// the requested identity for both increment and decrement. Use IdentityRequested
// to always use requested identity. NormalizeProvider and NormalizeModel can
// bucket or redact provider/model labels.
//
// Metric labels are intentionally bounded and content-free. Duration histograms
// use a lean label set; use aisdk_model_requests_total for detailed error type,
// status code, and finish reason breakdowns. Labels never include prompts,
// outputs, reasoning text, tool arguments/results, user IDs, tenant IDs, request
// or response IDs, headers, URLs, bodies, raw provider metadata, error messages,
// tool names, source URLs, filenames, or arbitrary per-request labels.
// ConstLabels should be restricted to process-level labels such as service,
// component, or environment.
//
// # Composition
//
// Prometheus middleware composes as ordinary middleware. Because the first
// middleware in middleware.Wrap is outermost, putting Prometheus inside Agent
// Observability (later in the slice) measures provider calls after hooks and
// recording have allowed or transformed the request. Putting Prometheus outside
// Agent Observability measures broader wrapped-model attempts, including hook
// denials and observability overhead. This package does not import
// middleware/agentobservability.
//
// # Hosted metrics
//
// This package records local client-side provider-call metrics. It does not
// enable, disable, or configure remote hosted-service metrics controls.
package prometheus
